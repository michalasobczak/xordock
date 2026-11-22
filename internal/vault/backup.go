package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Backup na kartę zapasową (S004 §5.5). Karta zapasowa przechowuje kopie
// udziałów B obiektów jednej migawki oraz rekord punktu odzyskiwania READY.
// Nie jest osobnym kluczem: to ten sam udział B w tej samej wersji (B-02).
//
// Kolejność (B-03, CG-10):
//  1. przypięcie punktu w indeksie sejfu (migawka nie zniknie w trakcie),
//  2. kopiowanie brakujących udziałów B do obj/ (tmp + rename),
//  3. utrwalenie i ponowna weryfikacja wszystkich wymaganych udziałów,
//  4. zapis rekordu points/<id> z pełnym fsync: dopiero teraz punkt jest READY,
//  5. oznaczenie przypięcia jako Ready w indeksie (informacja dla status).
//
// Przerwanie w dowolnym miejscu zostawia poprzednie punkty nietknięte:
// kopiowanie tylko dodaje pliki, nigdy ich nie usuwa ani nie nadpisuje.

const (
	pointsDir   = "points"
	stateReady  = "READY"
	spaceMargin = 1 << 20
)

var (
	ErrNoSpace  = errors.New("za mało miejsca na karcie zapasowej; poprzedni punkt odzyskiwania pozostaje bez zmian")
	ErrNoPoint  = errors.New("brak punktu odzyskiwania READY")
	ErrSameCard = errors.New("karta zapasowa musi być innym nośnikiem niż SSD i karta robocza")
)

// FreeSpace zwraca wolne miejsce w bajtach (podmieniane w testach, CG-11).
var FreeSpace = func(p string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// Point to rekord punktu odzyskiwania na karcie zapasowej. Zawiera tylko
// identyfikatory, liczby i czasy (D-04), bez nazw plików.
type Point struct {
	Format   int       `json:"format"`
	Vault    string    `json:"vault"`
	Media    string    `json:"media"`
	ID       string    `json:"id"`
	Snapshot string    `json:"snapshot"`
	Seq      uint64    `json:"seq"`
	Index    string    `json:"index"`
	Created  time.Time `json:"created"`
	Objects  int       `json:"objects"`
	Bytes    int64     `json:"bytes"`
	State    string    `json:"state"`
}

// prepareSpare zakłada albo sprawdza nagłówek karty zapasowej i zwraca
// identyfikator nośnika.
func (v *Vault) prepareSpare(spare string) (string, error) {
	if SameDeviceCheck {
		if err := os.MkdirAll(spare, 0o700); err != nil {
			return "", err
		}
		d := devOf(spare)
		if d == devOf(v.aRoot) || d == devOf(v.bRoot) {
			return "", ErrSameCard
		}
	}
	h, err := readHeader(spare, SideB)
	switch {
	case err == nil:
		if h.Vault != v.id.String() {
			return "", ErrWrongCard
		}
		if h.Role != roleBackup || h.Media == "" {
			return "", errors.New("wskazany katalog to karta robocza, nie zapasowa")
		}
		return h.Media, nil
	case errors.Is(err, fs.ErrNotExist):
		mid, err := newID()
		if err != nil {
			return "", err
		}
		h := header{Format: FormatVersion, Vault: v.id.String(), Side: string(SideB), Name: v.name,
			BlockSize: BlockSize, Created: time.Now().UTC(), Masks: v.MaskKind(), Role: roleBackup, Media: mid.String()}
		data, _ := json.MarshalIndent(h, "", "  ")
		if err := writeAtomic(filepath.Join(spare, headerFile), data, true); err != nil {
			return "", err
		}
		return h.Media, nil
	default:
		return "", fmt.Errorf("nagłówek karty zapasowej: %w", err)
	}
}

// Backup kopiuje na kartę zapasową udziały B migawki snapID (pusty: najnowszej)
// i zapisuje punkt odzyskiwania READY. Wymaga karty roboczej i zapasowej
// podłączonych jednocześnie.
func (v *Vault) Backup(spare, snapID string) (Point, error) {
	v.mu.Lock()
	if v.broken != nil {
		v.mu.Unlock()
		return Point{}, ErrBroken
	}
	snaps := v.idx.Snaps
	v.mu.Unlock()
	if len(snaps) == 0 {
		return Point{}, errors.New("brak migawek; najpierw: xordock snapshot create")
	}
	snap := snaps[len(snaps)-1]
	if snapID != "" {
		found := false
		for _, s := range snaps {
			if s.ID == snapID {
				snap, found = s, true
			}
		}
		if !found {
			return Point{}, ErrNoSnapshot
		}
	}
	media, err := v.prepareSpare(spare)
	if err != nil {
		return Point{}, err
	}

	// Lista wymaganych obiektów i miejsca (B-04: odmowa przed jakąkolwiek zmianą).
	ix, err := v.loadIndex(snap.Index)
	if err != nil {
		return Point{}, err
	}
	need := map[string]bool{snap.Index: true}
	ix.refs(need)
	ids := make([]ID, 0, len(need))
	for h := range need {
		id, err := ParseID(h)
		if err != nil {
			return Point{}, err
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	var missing []ID
	var missingBytes, total int64
	for _, id := range ids {
		st, err := os.Stat(objPath(v.bRoot, id))
		if err != nil {
			return Point{}, fmt.Errorf("udział B %s na karcie roboczej: %w", id, err)
		}
		total += st.Size()
		if _, err := os.Stat(objPath(spare, id)); err != nil {
			missing = append(missing, id)
			missingBytes += st.Size()
		}
	}
	free, err := FreeSpace(spare)
	if err != nil {
		return Point{}, err
	}
	if uint64(missingBytes+spaceMargin) > free {
		return Point{}, fmt.Errorf("%w (potrzeba %d B, wolne %d B)", ErrNoSpace, missingBytes+spaceMargin, free)
	}

	pid, err := newID()
	if err != nil {
		return Point{}, err
	}
	ref := PointRef{Media: media, Point: pid.String()}
	if err := v.Commit(func(ix *Index) error { return pinPoint(ix, snap.ID, ref) }); err != nil {
		return Point{}, fmt.Errorf("przypięcie migawki: %w", err)
	}

	// Kopiowanie bez blokady sejfu: obiekty migawki są niezmienne i przypięte.
	dirs := map[string]bool{}
	for _, id := range missing {
		if err := v.at("backup-copy"); err != nil {
			return Point{}, err
		}
		raw, err := os.ReadFile(objPath(v.bRoot, id))
		if err != nil {
			return Point{}, fmt.Errorf("udział B %s: %w", id, err)
		}
		if _, err := decodeShare(raw, SideB, v.id, id); err != nil {
			return Point{}, err
		}
		dst := objPath(spare, id)
		if err := writeAtomic(dst, raw, false); err != nil {
			return Point{}, fmt.Errorf("zapis na karcie zapasowej: %w", err)
		}
		dirs[filepath.Dir(dst)] = true
	}
	for d := range dirs {
		syncDir(d, false)
	}
	if err := barrier(spare); err != nil {
		return Point{}, err
	}
	if err := v.at("backup-verify"); err != nil {
		return Point{}, err
	}
	for _, id := range ids {
		raw, err := os.ReadFile(objPath(spare, id))
		if err == nil {
			_, err = decodeShare(raw, SideB, v.id, id)
		}
		if err != nil {
			return Point{}, fmt.Errorf("weryfikacja karty zapasowej %s: %w", id, err)
		}
		rawA, err := os.ReadFile(objPath(v.aRoot, id))
		if err == nil {
			_, err = decodeShare(rawA, SideA, v.id, id)
		}
		if err != nil {
			return Point{}, fmt.Errorf("udział A migawki %s: %w", id, err)
		}
	}
	if err := v.at("backup-ready"); err != nil {
		return Point{}, err
	}
	p := Point{Format: FormatVersion, Vault: v.id.String(), Media: media, ID: pid.String(), Snapshot: snap.ID,
		Seq: snap.Seq, Index: snap.Index, Created: time.Now().UTC(), Objects: len(ids), Bytes: total, State: stateReady}
	if err := writeAtomic(filepath.Join(spare, pointsDir, p.ID), seal(p), true); err != nil {
		return Point{}, fmt.Errorf("zapis punktu odzyskiwania: %w", err)
	}
	ref.Ready, ref.Time = true, p.Created
	if err := v.Commit(func(ix *Index) error { return pinPoint(ix, snap.ID, ref) }); err != nil {
		// Punkt jest READY na karcie; brak flagi w indeksie wpływa tylko na status.
		Logf("xordock: punkt %s READY, ale nie zapisano tego w indeksie: %v", p.ID, err)
	}
	return p, nil
}

// pinPoint dodaje albo aktualizuje przypięcie punktu w migawce.
func pinPoint(ix *Index, snapID string, ref PointRef) error {
	i, ok := ix.snap(snapID)
	if !ok {
		return ErrNoSnapshot
	}
	for j, r := range ix.Snaps[i].Points {
		if r.Media == ref.Media && r.Point == ref.Point {
			ix.Snaps[i].Points[j] = ref
			return nil
		}
	}
	ix.Snaps[i].Points = append(ix.Snaps[i].Points, ref)
	return nil
}

// ReadPoints zwraca poprawne punkty READY z karty zapasowej (najnowszy
// pierwszy) i opisy rekordów odrzuconych.
func ReadPoints(spare string, vault ID) ([]Point, []string) {
	var out []Point
	var bad []string
	entries, _ := os.ReadDir(filepath.Join(spare, pointsDir))
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(spare, pointsDir, e.Name()))
		var p Point
		if err == nil {
			err = unseal(data, &p)
		}
		switch {
		case err != nil:
			bad = append(bad, fmt.Sprintf("punkt %s: %v", e.Name(), err))
		case p.Vault != vault.String() || p.ID != e.Name():
			bad = append(bad, fmt.Sprintf("punkt %s: należy do innego sejfu albo ma złą nazwę", e.Name()))
		case p.State != stateReady:
			bad = append(bad, fmt.Sprintf("punkt %s: stan %q", e.Name(), p.State))
		default:
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seq != out[j].Seq {
			return out[i].Seq > out[j].Seq
		}
		return out[i].Created.After(out[j].Created)
	})
	return out, bad
}

// PruneBackup zostawia na karcie zapasowej tylko najnowszy punkt READY,
// usuwa starsze punkty i niepotrzebne już udziały, a potem zdejmuje ich
// przypięcia w sejfie. To jawna decyzja retencyjna użytkownika (B-04);
// nigdy nie usuwa ostatniego dobrego punktu.
func (v *Vault) PruneBackup(spare string) (removedPoints, removedObjects int, err error) {
	media, err := v.prepareSpare(spare)
	if err != nil {
		return 0, 0, err
	}
	points, _ := ReadPoints(spare, v.id)
	if len(points) == 0 {
		return 0, 0, ErrNoPoint
	}
	keep := points[0]
	ix, err := loadIndexFrom(v.aRoot, spare, v.id, keep.Index)
	if err != nil {
		return 0, 0, fmt.Errorf("najnowszy punkt %s nie daje się odczytać, nic nie usuwam: %w", keep.ID, err)
	}
	live := map[string]bool{keep.Index: true}
	ix.refs(live)
	// Najnowszy punkt musi być kompletny po obu stronach, zanim usuniemy
	// cokolwiek starszego (B-04, S005 R4-01).
	for h := range live {
		id, err := ParseID(h)
		if err != nil {
			return 0, 0, err
		}
		for _, s := range []struct {
			root string
			side Side
		}{{spare, SideB}, {v.aRoot, SideA}} {
			raw, err := os.ReadFile(objPath(s.root, id))
			if err == nil {
				_, err = decodeShare(raw, s.side, v.id, id)
			}
			if err != nil {
				return 0, 0, fmt.Errorf("najnowszy punkt %s jest niekompletny (%c/%s: %v), nic nie usuwam", keep.ID, s.side, id, err)
			}
		}
	}
	for _, p := range points[1:] {
		if err := os.Remove(filepath.Join(spare, pointsDir, p.ID)); err != nil {
			return removedPoints, 0, err
		}
		removedPoints++
	}
	syncDir(filepath.Join(spare, pointsDir), true)
	filepath.WalkDir(filepath.Join(spare, objDir), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !live[d.Name()] {
			if os.Remove(p) == nil {
				removedObjects++
			}
		}
		return nil
	})
	err = v.Commit(func(ix *Index) error {
		for i := range ix.Snaps {
			var kept []PointRef
			for _, r := range ix.Snaps[i].Points {
				if r.Media != media || r.Point == keep.ID {
					kept = append(kept, r)
				}
			}
			ix.Snaps[i].Points = kept
		}
		return nil
	})
	return removedPoints, removedObjects, err
}

// LastBackup zwraca najnowsze potwierdzone przypięcie (G-08) i migawkę.
func (v *Vault) LastBackup() (PointRef, Snapshot, bool) {
	var best PointRef
	var bestSnap Snapshot
	found := false
	for _, s := range v.Snapshots() {
		for _, r := range s.Points {
			if r.Ready && (!found || s.Seq > bestSnap.Seq || (s.Seq == bestSnap.Seq && r.Time.After(best.Time))) {
				best, bestSnap, found = r, s, true
			}
		}
	}
	return best, bestSnap, found
}
