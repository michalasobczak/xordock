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
func (v *Vault) prepareSpare(spare string) (string, error) { return v.prepareCopy(spare, SideB) }

// ErrACopyOnCard: kopia udziału A nie może leżeć na karcie (S008 G4).
var ErrACopyOnCard = errors.New("kopia udziału A nie może leżeć na karcie roboczej ani zapasowej")

// within: p leży w katalogu root albo jest nim.
func within(p, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// prepareCopy zakłada albo sprawdza nagłówek kopii strony side (B: karta
// zapasowa, S004; A: kopia w iCloud Drive, S008) i zwraca identyfikator nośnika.
func (v *Vault) prepareCopy(dst string, side Side) (string, error) {
	for _, r := range []string{v.aRoot, v.bRoot} {
		if within(dst, r) || within(r, dst) {
			if side == SideA {
				return "", ErrACopyOnCard
			}
			return "", ErrSameCard
		}
	}
	if SameDeviceCheck {
		if err := os.MkdirAll(dst, 0o700); err != nil {
			return "", err
		}
		d := devOf(dst)
		switch {
		case side == SideB && (d == devOf(v.aRoot) || d == devOf(v.bRoot)):
			return "", ErrSameCard
		case side == SideA && d == devOf(v.bRoot):
			return "", ErrACopyOnCard
		}
	}
	h, err := readHeader(dst, side)
	switch {
	case err == nil:
		if h.Vault != v.id.String() {
			return "", ErrWrongCard
		}
		if h.Role != roleBackup || h.Media == "" {
			if side == SideA {
				return "", errors.New("wskazany katalog to żywy udział A, nie kopia")
			}
			return "", errors.New("wskazany katalog to karta robocza, nie zapasowa")
		}
		return h.Media, nil
	case errors.Is(err, fs.ErrNotExist):
		mid, err := newID()
		if err != nil {
			return "", err
		}
		h := header{Format: FormatVersion, Vault: v.id.String(), Side: string(side), Name: v.name,
			BlockSize: BlockSize, Created: time.Now().UTC(), Masks: v.MaskKind(), Role: roleBackup, Media: mid.String()}
		data, _ := json.MarshalIndent(h, "", "  ")
		if err := writeAtomic(filepath.Join(dst, headerFile), data, true); err != nil {
			return "", err
		}
		return h.Media, nil
	default:
		return "", fmt.Errorf("nagłówek kopii %c: %w", side, err)
	}
}

// Backup kopiuje na kartę zapasową udziały B migawki snapID (pusty: najnowszej)
// i zapisuje punkt odzyskiwania READY. Wymaga karty roboczej i zapasowej
// podłączonych jednocześnie.
func (v *Vault) Backup(spare, snapID string) (Point, error) {
	ps, err := v.backup(snapID, []copyTarget{{root: spare, side: SideB}})
	if err != nil {
		return Point{}, err
	}
	return ps[0], nil
}

// BackupPair zapisuje jeden punkt odzyskiwania w dwóch miejscach: udziały B
// na kartę zapasową i udziały A tej samej migawki do kopii A (S008). Gdy
// kopia A się nie uda, punkt B zostaje READY, a błąd to mówi.
func (v *Vault) BackupPair(spare, aCopy, snapID string) (Point, Point, error) {
	if within(aCopy, spare) || within(spare, aCopy) {
		return Point{}, Point{}, ErrACopyOnCard
	}
	if SameDeviceCheck {
		for _, d := range []string{spare, aCopy} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return Point{}, Point{}, err
			}
		}
		if devOf(aCopy) == devOf(spare) {
			return Point{}, Point{}, ErrACopyOnCard
		}
	}
	ps, err := v.backup(snapID, []copyTarget{{root: spare, side: SideB}, {root: aCopy, side: SideA}})
	var pb, pa Point
	if len(ps) > 0 {
		pb = ps[0]
	}
	if len(ps) > 1 {
		pa = ps[1]
	}
	if err != nil && len(ps) == 1 {
		err = fmt.Errorf("punkt %s READY na karcie zapasowej, ale kopia udziału A nie powstała: %w", pb.ID, err)
	}
	return pb, pa, err
}

type copyTarget struct {
	root    string
	side    Side
	media   string
	missing []ID
	ref     PointRef
}

// stage: nazwy etapów dla haków testowych (B: jak w S004, A: acopy-*).
func (t copyTarget) stage(s string) string {
	if t.side == SideA {
		return "acopy-" + s
	}
	return "backup-" + s
}

// src: żywy udział strony kopii; other: żywy udział strony przeciwnej.
func (v *Vault) sideRoots(side Side) (src, other string, otherSide Side) {
	if side == SideA {
		return v.aRoot, v.bRoot, SideB
	}
	return v.bRoot, v.aRoot, SideA
}

func (v *Vault) backup(snapID string, targets []copyTarget) ([]Point, error) {
	v.mu.Lock()
	if v.broken != nil {
		v.mu.Unlock()
		return nil, ErrBroken
	}
	snaps := v.idx.Snaps
	v.mu.Unlock()
	if len(snaps) == 0 {
		return nil, errors.New("brak migawek; najpierw: xordock snapshot create")
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
			return nil, ErrNoSnapshot
		}
	}
	for i := range targets {
		media, err := v.prepareCopy(targets[i].root, targets[i].side)
		if err != nil {
			return nil, err
		}
		targets[i].media = media
	}

	// Lista wymaganych obiektów i miejsca (B-04: odmowa przed jakąkolwiek zmianą).
	ix, err := v.loadIndex(snap.Index)
	if err != nil {
		return nil, err
	}
	need := map[string]bool{snap.Index: true}
	ix.refs(need)
	ids := make([]ID, 0, len(need))
	for h := range need {
		id, err := ParseID(h)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	sizes := map[ID]int64{}
	var total int64
	for i := range targets {
		t := &targets[i]
		src, _, _ := v.sideRoots(t.side)
		var missingBytes int64
		for _, id := range ids {
			st, err := os.Stat(objPath(src, id))
			if err != nil {
				return nil, fmt.Errorf("udział %c %s: %w", t.side, id, err)
			}
			if t.side == SideB {
				sizes[id] = st.Size()
			}
			if _, err := os.Stat(objPath(t.root, id)); err != nil {
				t.missing = append(t.missing, id)
				missingBytes += st.Size()
			}
		}
		free, err := FreeSpace(t.root)
		if err != nil {
			return nil, err
		}
		if uint64(missingBytes+spaceMargin) > free {
			where := "na karcie zapasowej"
			if t.side == SideA {
				where = "na celu kopii udziału A"
			}
			return nil, fmt.Errorf("%w (%s potrzeba %d B, wolne %d B)", ErrNoSpace, where, missingBytes+spaceMargin, free)
		}
	}
	for _, n := range sizes {
		total += n
	}

	pid, err := newID()
	if err != nil {
		return nil, err
	}
	for i := range targets {
		targets[i].ref = PointRef{Media: targets[i].media, Point: pid.String()}
	}
	if err := v.Commit(func(ix *Index) error {
		for _, t := range targets {
			if err := pinPoint(ix, snap.ID, t.ref); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("przypięcie migawki: %w", err)
	}

	var out []Point
	for _, t := range targets {
		p, err := v.copySide(t, snap, pid, ids, total)
		if err != nil {
			return out, err
		}
		out = append(out, p)
	}
	return out, nil
}

// copySide kopiuje brakujące udziały strony t.side migawki, weryfikuje cel
// i stronę przeciwną, zapisuje rekord punktu READY i oznacza przypięcie.
// Kopiowanie bez blokady sejfu: obiekty migawki są niezmienne i przypięte.
func (v *Vault) copySide(t copyTarget, snap Snapshot, pid ID, ids []ID, total int64) (Point, error) {
	src, other, otherSide := v.sideRoots(t.side)
	dirs := map[string]bool{}
	for _, id := range t.missing {
		if err := v.at(t.stage("copy")); err != nil {
			return Point{}, err
		}
		raw, err := os.ReadFile(objPath(src, id))
		if err != nil {
			return Point{}, fmt.Errorf("udział %c %s: %w", t.side, id, err)
		}
		// Do kopii trafia wyłącznie poprawny udział tej strony (S008 G3).
		if _, err := decodeShare(raw, t.side, v.id, id); err != nil {
			return Point{}, err
		}
		dst := objPath(t.root, id)
		if err := writeAtomic(dst, raw, false); err != nil {
			return Point{}, fmt.Errorf("zapis kopii %c: %w", t.side, err)
		}
		dirs[filepath.Dir(dst)] = true
	}
	for d := range dirs {
		syncDir(d, false)
	}
	if err := barrier(t.root); err != nil {
		return Point{}, err
	}
	if err := v.at(t.stage("verify")); err != nil {
		return Point{}, err
	}
	for _, id := range ids {
		raw, err := os.ReadFile(objPath(t.root, id))
		if err == nil {
			_, err = decodeShare(raw, t.side, v.id, id)
		}
		if err != nil {
			return Point{}, fmt.Errorf("weryfikacja kopii %c %s: %w", t.side, id, err)
		}
		rawO, err := os.ReadFile(objPath(other, id))
		if err == nil {
			_, err = decodeShare(rawO, otherSide, v.id, id)
		}
		if err != nil {
			return Point{}, fmt.Errorf("udział %c migawki %s: %w", otherSide, id, err)
		}
	}
	if err := v.at(t.stage("ready")); err != nil {
		return Point{}, err
	}
	p := Point{Format: FormatVersion, Vault: v.id.String(), Media: t.media, ID: pid.String(), Snapshot: snap.ID,
		Seq: snap.Seq, Index: snap.Index, Created: time.Now().UTC(), Objects: len(ids), Bytes: total, State: stateReady}
	if err := writeAtomic(filepath.Join(t.root, pointsDir, p.ID), seal(p), true); err != nil {
		return Point{}, fmt.Errorf("zapis punktu odzyskiwania: %w", err)
	}
	ref := t.ref
	ref.Ready, ref.Time = true, p.Created
	if err := v.Commit(func(ix *Index) error { return pinPoint(ix, snap.ID, ref) }); err != nil {
		// Punkt jest READY na nośniku; brak flagi w indeksie wpływa tylko na status.
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
	return v.pruneCopy(spare, SideB)
}

// PruneACopy: to samo dla kopii udziału A (S008 K-03).
func (v *Vault) PruneACopy(aCopy string) (removedPoints, removedObjects int, err error) {
	return v.pruneCopy(aCopy, SideA)
}

func (v *Vault) pruneCopy(dst string, side Side) (removedPoints, removedObjects int, err error) {
	media, err := v.prepareCopy(dst, side)
	if err != nil {
		return 0, 0, err
	}
	points, _ := ReadPoints(dst, v.id)
	if len(points) == 0 {
		return 0, 0, ErrNoPoint
	}
	keep := points[0]
	_, other, otherSide := v.sideRoots(side)
	aSrc, bSrc := other, dst
	if side == SideA {
		aSrc, bSrc = dst, other
	}
	ix, err := loadIndexFrom(aSrc, bSrc, v.id, keep.Index)
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
		}{{dst, side}, {other, otherSide}} {
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
		if err := os.Remove(filepath.Join(dst, pointsDir, p.ID)); err != nil {
			return removedPoints, 0, err
		}
		removedPoints++
	}
	syncDir(filepath.Join(dst, pointsDir), true)
	filepath.WalkDir(filepath.Join(dst, objDir), func(p string, d fs.DirEntry, err error) error {
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
	return lastReady(v.Snapshots())
}

// lastReady wybiera najnowszy punkt READY spośród migawek.
func lastReady(snaps []Snapshot) (PointRef, Snapshot, bool) {
	var best PointRef
	var bestSnap Snapshot
	found := false
	for _, s := range snaps {
		for _, r := range s.Points {
			if r.Ready && (!found || s.Seq > bestSnap.Seq || (s.Seq == bestSnap.Seq && r.Time.After(best.Time))) {
				best, bestSnap, found = r, s, true
			}
		}
	}
	return best, bestSnap, found
}
