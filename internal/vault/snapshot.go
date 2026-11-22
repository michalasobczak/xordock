package vault

import (
	"errors"
	"path"
	"sort"
	"time"

	"xordock/internal/split"
)

// xordock share snapshots (S004 §5.4). A snapshot is an immutable pointer to a
// committed index: it pins its A and B objects against GC. It does not take
// an APFS snapshot and does not copy plain data (CG-06). The snapshot list lives
// in the index, so it is XOR-split and committed like any change.

var (
	ErrNoSnapshot = errors.New("no such snapshot")
	ErrPinned     = errors.New("snapshot is pinned by a recovery point on the spare card (first run: xordock backup prune)")
)

// PointRef links a snapshot to a recovery point on the spare card.
// We record the pin before copying (Ready=false), and Ready=true after
// the READY point is written. A pin without a point only pauses GC.
type PointRef struct {
	Media string    `json:"m"`
	Point string    `json:"p"`
	Ready bool      `json:"r,omitempty"`
	Time  time.Time `json:"t,omitempty"`
}

type Snapshot struct {
	ID     string     `json:"id"`
	Seq    uint64     `json:"q"` // generation (commit number)
	Index  string     `json:"x"` // index object of this generation
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

// CreateSnapshot pins the current committed state as a snapshot.
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

// Snapshots returns the snapshots from the oldest.
func (v *Vault) Snapshots() []Snapshot {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.idx.clone().Snaps
}

// DeleteSnapshot deletes a snapshot if no recovery point pins
// it. GC deletes its objects in the same commit.
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

// SnapshotView gives read access to files from a snapshot (CG-07).
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

// Changes returns the paths that differ between a snapshot and the current state
// (G-08: what would be lost after losing the working card).
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

// FileReader reads a file tree: the current state, a snapshot or a recovery
// point. Used by export and by restore to a new vault.
type FileReader interface {
	Paths() []string
	Stat(p string) (Entry, bool)
	GetFile(p string) ([]byte, error)
}

// View is an immutable view of an index with an object-joining function.
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

// List returns the direct children of a directory in the view.
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

// readEntry joins a file from its blocks and checks the size. The caller wipes the result.
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
