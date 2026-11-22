package vault

import (
	"errors"
	"path"
	"sort"
	"time"

	"xordock/internal/split"
)

// Migawki udziałów xordocka (S004 §5.4). Migawka to niezmienny wskaźnik na
// zatwierdzony indeks: przypina jego obiekty A i B przed GC. Nie robi
// migawki APFS i nie kopiuje jawnych danych (CG-06). Lista migawek leży
// w indeksie, więc jest dzielona XOR i zatwierdzana jak każda zmiana.

var (
	ErrNoSnapshot = errors.New("nie ma takiej migawki")
	ErrPinned     = errors.New("migawka jest przypięta przez punkt odzyskiwania na karcie zapasowej (najpierw: xordock backup prune)")
)

// PointRef wiąże migawkę z punktem odzyskiwania na karcie zapasowej.
// Przypięcie zapisujemy przed kopiowaniem (Ready=false), a Ready=true po
// zapisaniu punktu READY. Przypięcie bez punktu tylko wstrzymuje GC.
type PointRef struct {
	Media string    `json:"m"`
	Point string    `json:"p"`
	Ready bool      `json:"r,omitempty"`
	Time  time.Time `json:"t,omitempty"`
}

type Snapshot struct {
	ID     string     `json:"id"`
	Seq    uint64     `json:"q"` // generacja (numer commitu)
	Index  string     `json:"x"` // obiekt indeksu tej generacji
	Time   time.Time  `json:"t"`
	Points []PointRef `json:"p,omitempty"`
}

func (ix *Index) snap(id string) (int, bool) {
	for i, s := range ix.Snaps {
		if s.ID == id {
			return i, true
		}
	}
	return 0, false
}

// CreateSnapshot przypina bieżący zatwierdzony stan jako migawkę.
func (v *Vault) CreateSnapshot() (Snapshot, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.broken != nil {
		return Snapshot{}, ErrBroken
	}
	sid, err := newID()
	if err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{ID: sid.String(), Seq: v.cur.Seq, Index: v.cur.Index, Time: time.Now().UTC()}
	refs := map[string]bool{v.cur.Index: true}
	v.idx.refs(refs)
	if v.snapRefs == nil {
		v.snapRefs = map[string]map[string]bool{}
	}
	v.snapRefs[s.Index] = refs
	nix := v.idx.clone()
	nix.Snaps = append(nix.Snaps, s)
	if err := v.commitIndex(nix); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

// Snapshots zwraca migawki od najstarszej.
func (v *Vault) Snapshots() []Snapshot {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.idx.clone().Snaps
}

// DeleteSnapshot usuwa migawkę, jeśli nie przypina jej żaden punkt
// odzyskiwania. Jej obiekty usuwa GC w tym samym commicie.
func (v *Vault) DeleteSnapshot(id string) error {
	return v.Commit(func(ix *Index) error {
		i, ok := ix.snap(id)
		if !ok {
			return ErrNoSnapshot
		}
		if len(ix.Snaps[i].Points) > 0 {
			return ErrPinned
		}
		ix.Snaps = append(ix.Snaps[:i:i], ix.Snaps[i+1:]...)
		return nil
	})
}

// SnapshotView daje odczyt plików z migawki (CG-07).
func (v *Vault) SnapshotView(id string) (*View, error) {
	v.mu.Lock()
	i, ok := v.idx.snap(id)
	var s Snapshot
	if ok {
		s = v.idx.Snaps[i]
	}
	v.mu.Unlock()
	if !ok {
		return nil, ErrNoSnapshot
	}
	ix, err := v.loadIndex(s.Index)
	if err != nil {
		return nil, err
	}
	return &View{ix: ix, read: v.readShares}, nil
}

// Changes zwraca ścieżki, które różnią się między migawką a stanem bieżącym
// (G-08: co przepadnie po utracie karty roboczej).
func (v *Vault) Changes(id string) ([]string, error) {
	old, err := v.SnapshotView(id)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	cur := v.idx.clone()
	v.mu.Unlock()
	return diffIndex(old.ix, cur), nil
}

func diffIndex(a, b *Index) []string {
	var out []string
	same := func(x, y *Entry) bool {
		if x.Dir != y.Dir || x.Size != y.Size || len(x.Blocks) != len(y.Blocks) {
			return false
		}
		for i := range x.Blocks {
			if x.Blocks[i] != y.Blocks[i] {
				return false
			}
		}
		return true
	}
	for p, e := range b.Entries {
		if o, ok := a.Entries[p]; !ok || !same(o, e) {
			out = append(out, p)
		}
	}
	for p := range a.Entries {
		if _, ok := b.Entries[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// FileReader to odczyt drzewa plików: stan bieżący, migawka albo punkt
// odzyskiwania. Używany przez eksport i odtwarzanie do nowego sejfu.
type FileReader interface {
	Paths() []string
	Stat(p string) (Entry, bool)
	GetFile(p string) ([]byte, error)
}

// View to niezmienny widok indeksu z funkcją składania obiektów.
type View struct {
	ix   *Index
	read func(ID) ([]byte, error)
}

func (w *View) Paths() []string {
	out := make([]string, 0, len(w.ix.Entries))
	for p := range w.ix.Entries {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (w *View) Stat(p string) (Entry, bool) {
	e, ok := w.ix.Entries[Clean(p)]
	if !ok {
		return Entry{}, false
	}
	c := *e
	c.Blocks = append([]string(nil), e.Blocks...)
	return c, true
}

func (w *View) GetFile(p string) ([]byte, error) {
	e, ok := w.Stat(p)
	if !ok {
		return nil, ErrNotExist
	}
	return readEntry(e, func(oid string) ([]byte, error) {
		id, err := ParseID(oid)
		if err != nil {
			return nil, err
		}
		return w.read(id)
	})
}

// List zwraca bezpośrednie dzieci katalogu w widoku.
func (w *View) List(dir string) []DirItem {
	dir = Clean(dir)
	var out []DirItem
	for p, e := range w.ix.Entries {
		if p != "/" && path.Dir(p) == dir {
			out = append(out, DirItem{Name: path.Base(p), Entry: *e})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// readEntry składa plik z bloków i sprawdza rozmiar. Wywołujący czyści wynik.
func readEntry(e Entry, read func(string) ([]byte, error)) ([]byte, error) {
	if e.Dir {
		return nil, ErrIsDir
	}
	out := make([]byte, 0, e.Size)
	for _, b := range e.Blocks {
		blk, err := read(b)
		if err != nil {
			split.Wipe(out)
			return nil, err
		}
		out = append(out, blk...)
		split.Wipe(blk)
	}
	if int64(len(out)) != e.Size {
		split.Wipe(out)
		return nil, ErrCorrupt
	}
	return out, nil
}
