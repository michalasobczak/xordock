package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"xordock/internal/split"
)

// Odzyskiwanie po utracie karty roboczej (S004 §5.6). Składamy obiekty
// z udziału A na SSD (przypiętego przez migawkę) i kopii udziału B z karty
// zapasowej. Tylko dla punktu READY i tylko obiekty jego indeksu: nigdy nie
// mieszamy A_G z B_(G+N) (R-02, CG-09). Nic nie zapisujemy na karcie
// zapasowej ani w udziale A (R-03); jedynym plikiem jest blokada sejfu.

type Recovery struct {
	aRoot, spare string
	id           ID
	Media        string
	lock         *os.File
	aCopy, bCopy bool // strona jest kopią (rola backup) z rekordami punktów
}

// OpenRecovery otwiera odzyskiwanie z udziału A (żywego albo kopii z iCloud,
// S008) i udziału B (karty zapasowej albo roboczej). Co najmniej jedna strona
// musi być kopią z rekordami punktów. Kopie nie są blokowane ani zmieniane.
func OpenRecovery(aRoot, spare string) (*Recovery, error) {
	hA, err := readHeader(aRoot, SideA)
	if err != nil {
		return nil, fmt.Errorf("udział A: %w", err)
	}
	hS, err := readHeader(spare, SideB)
	if err != nil {
		return nil, fmt.Errorf("karta zapasowa: %w", err)
	}
	if hS.Role != roleBackup && hA.Role != roleBackup {
		return nil, errors.New("to karta robocza, nie zapasowa; odzyskiwanie nie jest potrzebne")
	}
	if hA.Vault != hS.Vault {
		return nil, ErrWrongCard
	}
	id, err := ParseID(hA.Vault)
	if err != nil {
		return nil, err
	}
	r := &Recovery{aRoot: aRoot, spare: spare, id: id, Media: hS.Media, aCopy: hA.Role == roleBackup, bCopy: hS.Role == roleBackup}
	if !r.aCopy {
		if r.lock, err = lockRoot(aRoot); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Recovery) Close() {
	if r.lock != nil {
		r.lock.Close()
		r.lock = nil
	}
}

// Points zwraca punkty READY (najnowszy pierwszy) i opisy odrzuconych.
// Gdy obie strony są kopiami, tylko punkty obecne na obu.
func (r *Recovery) Points() ([]Point, []string) {
	switch {
	case r.aCopy && r.bCopy:
		pb, bad := ReadPoints(r.spare, r.id)
		pa, badA := ReadPoints(r.aRoot, r.id)
		inA := map[string]bool{}
		for _, p := range pa {
			inA[p.ID] = true
		}
		var both []Point
		for _, p := range pb {
			if inA[p.ID] {
				both = append(both, p)
			} else {
				bad = append(bad, fmt.Sprintf("punkt %s: brak kopii udziału A", p.ID))
			}
		}
		return both, append(bad, badA...)
	case r.bCopy:
		return ReadPoints(r.spare, r.id)
	default:
		return ReadPoints(r.aRoot, r.id)
	}
}

func (r *Recovery) point(id string) (Point, error) {
	points, _ := r.Points()
	if len(points) == 0 {
		return Point{}, ErrNoPoint
	}
	if id == "" {
		return points[0], nil
	}
	for _, p := range points {
		if p.ID == id {
			return p, nil
		}
	}
	return Point{}, fmt.Errorf("%w o identyfikatorze %s", ErrNoPoint, id)
}

// View otwiera stan katalogu z punktu odzyskiwania (pusty id: najnowszy).
func (r *Recovery) View(pointID string) (*View, Point, error) {
	p, err := r.point(pointID)
	if err != nil {
		return nil, p, err
	}
	ix, err := loadIndexFrom(r.aRoot, r.spare, r.id, p.Index)
	if err != nil {
		return nil, p, fmt.Errorf("indeks punktu %s: %w", p.ID, err)
	}
	allowed := map[string]bool{p.Index: true}
	ix.refs(allowed)
	read := func(oid ID) ([]byte, error) {
		if !allowed[oid.String()] {
			return nil, fmt.Errorf("%w: obiekt %s spoza punktu odzyskiwania", ErrCorrupt, oid)
		}
		return readPair(r.aRoot, r.spare, r.id, oid)
	}
	return &View{ix: ix, read: read}, p, nil
}

// Verify składa w RAM każdy plik punktu i sprawdza kompletność (R-03).
func (r *Recovery) Verify(pointID string) (Point, int, error) {
	w, p, err := r.View(pointID)
	if err != nil {
		return p, 0, err
	}
	n := 0
	for _, path := range w.Paths() {
		e, _ := w.Stat(path)
		if e.Dir {
			continue
		}
		data, err := w.GetFile(path)
		if err != nil {
			return p, n, fmt.Errorf("plik nr %d: %w", e.Ino, err)
		}
		split.Wipe(data)
		n++
	}
	return p, n, nil
}

// ExportTo zapisuje jawne pliki z czytnika do nowego katalogu. Pliki
// wynikowe NIE są chronione (rollback S004 §8).
func ExportTo(r FileReader, dir string) (int, error) {
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("%s już istnieje", dir)
	}
	n := 0
	for _, p := range r.Paths() {
		e, _ := r.Stat(p)
		if p != Clean(p) {
			return n, fmt.Errorf("%w: niekanoniczna ścieżka", ErrCorrupt)
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if rel, err := filepath.Rel(dir, dst); err != nil || strings.HasPrefix(rel, "..") {
			return n, fmt.Errorf("%w: ścieżka poza katalogiem eksportu", ErrCorrupt)
		}
		if e.Dir {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return n, err
			}
			continue
		}
		data, err := r.GetFile(p)
		if err != nil {
			return n, err
		}
		err = os.WriteFile(dst, data, 0o600)
		split.Wipe(data)
		if err != nil {
			return n, err
		}
		os.Chtimes(dst, e.ModTime, e.ModTime)
		n++
	}
	return n, nil
}

// CopyInto przepisuje drzewo plików do innego sejfu. Każdy blok dostaje
// nową maskę, więc zgubiona karta nie pasuje do nowych udziałów (R-04).
func CopyInto(r FileReader, dst *Vault) (int, error) {
	n := 0
	for _, p := range r.Paths() {
		if p == "/" {
			continue
		}
		e, _ := r.Stat(p)
		if e.Dir {
			if err := dst.Mkdir(p); err != nil {
				return n, err
			}
			continue
		}
		data, err := r.GetFile(p)
		if err != nil {
			return n, err
		}
		err = dst.PutFile(p, data)
		split.Wipe(data)
		if err != nil {
			return n, err
		}
		if de, ok := dst.Stat(p); ok {
			dst.SetModTime(de.Ino, e.ModTime)
		}
		n++
	}
	return n, nil
}
