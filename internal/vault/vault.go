package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"xordock/internal/split"
)

// Logf is the package's only log channel. We log only IDs, numbers
// and errors, never file content, file names or masks (FR-15, CG-09).
var Logf = log.Printf

// Entry describes a file or directory in the index. The index (with file names) is
// stored as an XOR-split object, so names are not in plain form on A.
type Entry struct {
	Ino     uint64    `json:"i"` // stable file number (survives renames); 1 = root directory
	Dir     bool      `json:"d,omitempty"`
	Size    int64     `json:"s"`
	ModTime time.Time `json:"t"`
	Blocks  []string  `json:"b,omitempty"`
}

type Index struct {
	Entries map[string]*Entry `json:"e"`
	NextIno uint64            `json:"n"`
	Snaps   []Snapshot        `json:"s,omitempty"` // snapshots (S004 §5.4), also XOR-split
}

func newIndex() *Index {
	return &Index{Entries: map[string]*Entry{"/": {Ino: 1, Dir: true, ModTime: time.Now().UTC()}}, NextIno: 2}
}

// allocIno allocates a new file number.
func (ix *Index) allocIno() uint64 {
	if ix.NextIno < 2 {
		ix.NextIno = 2
	}
	n := ix.NextIno
	ix.NextIno++
	return n
}

// ensureInos assigns numbers to entries from indexes created before numbers existed
// (deterministically, in path order).
func (ix *Index) ensureInos() {
	ix.Entries["/"].Ino = 1
	var missing []string
	for p, e := range ix.Entries {
		if e.Ino == 0 {
			missing = append(missing, p)
		}
		if e.Ino >= ix.NextIno {
			ix.NextIno = e.Ino + 1
		}
	}
	sort.Strings(missing)
	for _, p := range missing {
		ix.Entries[p].Ino = ix.allocIno()
	}
}

func (ix *Index) pathOf(ino uint64) (string, bool) {
	for p, e := range ix.Entries {
		if e.Ino == ino {
			return p, true
		}
	}
	return "", false
}

func (ix *Index) clone() *Index {
	c := &Index{Entries: make(map[string]*Entry, len(ix.Entries)), NextIno: ix.NextIno}
	for k, e := range ix.Entries {
		ce := *e
		ce.Blocks = append([]string(nil), e.Blocks...)
		c.Entries[k] = &ce
	}
	for _, sn := range ix.Snaps {
		sn.Points = append([]PointRef(nil), sn.Points...)
		c.Snaps = append(c.Snaps, sn)
	}
	return c
}

func (ix *Index) refs(into map[string]bool) {
	if ix == nil {
		return
	}
	for _, e := range ix.Entries {
		for _, b := range e.Blocks {
			into[b] = true
		}
	}
}

type Options struct {
	// Masks: the mask source; split.CSPRNG by default.
	Masks split.MaskSource
	// Hook: fault injection point in tests (CG-07). Returning an error
	// aborts the operation as if the process died at that stage.
	Hook func(stage string) error
}

type Vault struct {
	mu       sync.Mutex
	aRoot    string
	bRoot    string
	id       ID
	name     string
	masks    split.MaskSource
	hook     func(string) error
	cur      commitRec
	prev     commitRec
	idx      *Index
	prevIdx  *Index
	broken   error
	lock     *os.File
	inos     map[uint64]string          // ino → path for the current index
	dirty    map[string]bool            // directories with new objects, to sync on commit
	snapRefs map[string]map[string]bool // snapshot index → objects it pins
	Recovery string                     // description of recovery on open, empty when none was needed
}

func (o Options) masksOrDefault() split.MaskSource {
	if o.Masks != nil {
		return o.Masks
	}
	return split.CSPRNG{}
}

// Close releases the vault lock.
func (v *Vault) Close() {
	if v.lock != nil {
		v.lock.Close()
		v.lock = nil
	}
}

func (v *Vault) ID() ID           { return v.id }
func (v *Vault) Name() string     { return v.name }
func (v *Vault) BRoot() string    { return v.bRoot }
func (v *Vault) ARoot() string    { return v.aRoot }
func (v *Vault) Seq() uint64      { v.mu.Lock(); defer v.mu.Unlock(); return v.cur.Seq }
func (v *Vault) MaskKind() string { return split.Kind(v.masks) }

func (v *Vault) at(stage string) error {
	if v.hook != nil {
		return v.hook(stage)
	}
	return nil
}

// fail switches the vault to the error state: no further commit will succeed (FR-18).
func (v *Vault) fail(err error) error {
	if v.broken == nil {
		v.broken = err
		Logf("xordock: vault %s in error state: %v", v.id, err)
	}
	return err
}

// Broken returns the error that switched the vault to the error state, or nil.
func (v *Vault) Broken() error { v.mu.Lock(); defer v.mu.Unlock(); return v.broken }

// MarkBroken lets the mount layer report the loss of the card.
func (v *Vault) MarkBroken(err error) { v.mu.Lock(); defer v.mu.Unlock(); v.fail(err) }

// Create initializes a new vault. It refuses if either side already has
// a committed state. An interrupted initialization is completed by Open (FR-06).
func Create(aRoot, bRoot, name string, opt Options) (*Vault, error) {
	for _, r := range []string{aRoot, bRoot} {
		if _, err := os.Stat(filepath.Join(r, superFile)); err == nil {
			return nil, fmt.Errorf("%s: vault already exists", r)
		}
	}
	if same, _ := sameDevice(aRoot, bRoot); same {
		return nil, errors.New("shares A and B must be on different devices")
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, s := range []struct {
		root string
		side Side
	}{{aRoot, SideA}, {bRoot, SideB}} {
		h := header{Format: FormatVersion, Vault: id.String(), Side: string(s.side), Name: name,
			BlockSize: BlockSize, Created: now, Masks: split.Kind(opt.masksOrDefault())}
		data, _ := json.MarshalIndent(h, "", "  ")
		if err := writeAtomic(filepath.Join(s.root, headerFile), data, true); err != nil {
			return nil, err
		}
	}
	v := &Vault{aRoot: aRoot, bRoot: bRoot, id: id, name: name, masks: opt.masksOrDefault(), hook: opt.Hook, idx: newIndex()}
	if err := v.commitIndex(v.idx.clone()); err != nil {
		return nil, err
	}
	return v, nil
}

// SameDeviceCheck can be disabled in tests, where A and B are in one temporary directory.
var SameDeviceCheck = true

func sameDevice(a, b string) (bool, error) {
	if !SameDeviceCheck {
		return false, nil
	}
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return false, err
		}
	}
	return devOf(a) == devOf(b), nil
}

// ReadVaultID returns the vault ID from the share A header.
func ReadVaultID(aRoot string) (ID, string, error) {
	h, err := readHeader(aRoot, SideA)
	if err != nil {
		return ID{}, "", err
	}
	id, err := ParseID(h.Vault)
	return id, h.Name, err
}

// ReadCopyID returns the vault ID and name from the header of a share A copy
// (backup role, S008). A live share A is rejected.
func ReadCopyID(dir string) (ID, string, error) {
	h, err := readHeader(dir, SideA)
	if err != nil {
		return ID{}, "", err
	}
	if h.Role != roleBackup {
		return ID{}, "", errors.New("this is a live share A, not a copy")
	}
	id, err := ParseID(h.Vault)
	return id, h.Name, err
}

// Open opens a vault, reconciles the superblocks after a possible crash and cleans up
// orphaned objects.
func Open(aRoot, bRoot string, opt Options) (*Vault, error) {
	hA, err := readHeader(aRoot, SideA)
	if err != nil {
		return nil, fmt.Errorf("share A: %w", err)
	}
	hB, err := readHeader(bRoot, SideB)
	if err != nil {
		return nil, fmt.Errorf("share B: %w", err)
	}
	if hB.Role == roleBackup {
		return nil, ErrSpareCard
	}
	if hA.Vault != hB.Vault {
		return nil, ErrWrongCard
	}
	if hA.BlockSize != BlockSize {
		return nil, fmt.Errorf("%w: block size %d, supported %d", ErrCorrupt, hA.BlockSize, BlockSize)
	}
	id, err := ParseID(hA.Vault)
	if err != nil {
		return nil, err
	}
	v := &Vault{aRoot: aRoot, bRoot: bRoot, id: id, name: hA.Name, masks: opt.masksOrDefault(), hook: opt.Hook}
	if v.lock, err = lockRoot(aRoot); err != nil {
		return nil, err
	}
	cleanTmp(aRoot)
	cleanTmp(bRoot)
	if err := v.reconcile(); err != nil {
		v.Close()
		return nil, err
	}
	if v.idx, err = v.loadIndex(v.cur.Index); err != nil {
		v.Close()
		return nil, fmt.Errorf("index: %w", err)
	}
	if v.prev.Seq > 0 {
		if v.prevIdx, err = v.loadIndex(v.prev.Index); err != nil {
			// The previous state is only a fallback point; the vault works without it.
			Logf("xordock: previous index unavailable (%v), skipping", err)
			v.prev, v.prevIdx = commitRec{}, nil
		}
	}
	v.gcAll()
	return v, nil
}

func (v *Vault) reconcile() error {
	sA, errA := readSuper(v.aRoot, SideA, v.id)
	sB, errB := readSuper(v.bRoot, SideB, v.id)
	switch {
	case errA == nil && errB == nil && sA.Cur == sB.Cur:
		v.cur = sA.Cur
		if sA.Prev == sB.Prev {
			v.prev = sA.Prev
		}
		return nil
	case errA == nil && errB == nil && sA.Cur.Seq == sB.Cur.Seq+1 && sA.Prev == sB.Cur:
		// Crash between writing superblock A and B.
		return v.finishOrRollback(sA, &sB)
	case errA == nil && errors.Is(errB, fs.ErrNotExist) && sA.Cur.Seq == 1:
		// Crash during initialization.
		return v.finishOrRollback(sA, nil)
	case errA != nil:
		return fmt.Errorf("superblock A: %w", errA)
	case errB != nil:
		return fmt.Errorf("superblock B: %w", errB)
	}
	return ErrInconsistent
}

// finishOrRollback completes a commit if all its objects are complete
// on both sides; otherwise it returns to the last common state.
func (v *Vault) finishOrRollback(sA superblock, sB *superblock) error {
	var older *Index
	if sB != nil {
		var err error
		if older, err = v.loadIndex(sB.Cur.Index); err != nil {
			return fmt.Errorf("%w: %v", ErrInconsistent, err)
		}
	}
	if v.commitComplete(sA.Cur, older) {
		if err := v.writeSuper(v.bRoot, SideB, sA.Cur, sA.Prev); err != nil {
			return err
		}
		v.cur, v.prev = sA.Cur, sA.Prev
		v.Recovery = fmt.Sprintf("completed interrupted commit %d", sA.Cur.Seq)
	} else {
		if sB == nil {
			return fmt.Errorf("%w: unfinished initialization", ErrInconsistent)
		}
		if err := v.writeSuper(v.aRoot, SideA, sB.Cur, sB.Prev); err != nil {
			return err
		}
		v.cur, v.prev = sB.Cur, sB.Prev
		v.Recovery = fmt.Sprintf("rolled back interrupted commit %d, state %d", sA.Cur.Seq, sB.Cur.Seq)
	}
	Logf("xordock: %s", v.Recovery)
	return nil
}

func (v *Vault) commitComplete(rec commitRec, older *Index) bool {
	ix, err := v.loadIndex(rec.Index)
	if err != nil {
		return false
	}
	known := map[string]bool{}
	older.refs(known)
	for _, e := range ix.Entries {
		for _, b := range e.Blocks {
			if known[b] {
				continue
			}
			oid, err := ParseID(b)
			if err != nil {
				return false
			}
			if _, err := v.readShares(oid); err != nil {
				return false
			}
		}
	}
	return true
}

func (v *Vault) writeSuper(root string, side Side, cur, prev commitRec) error {
	s := superblock{Format: FormatVersion, Vault: v.id.String(), Side: string(side), Cur: cur, Prev: prev}
	return writeAtomic(filepath.Join(root, superFile), encodeSuper(s), true)
}

func (v *Vault) loadIndex(oid string) (*Index, error) {
	return loadIndexFrom(v.aRoot, v.bRoot, v.id, oid)
}

// readShares reads both shares of an object, verifies them and returns the plain data.
// The caller is responsible for split.Wipe on the result.
func (v *Vault) readShares(oid ID) ([]byte, error) {
	return readPair(v.aRoot, v.bRoot, v.id, oid)
}

// readPair joins an object from share A (aRoot) and B (bRoot: working or
// spare card). A missing or damaged side is an error, never
// substitute data (G-03, R-02).
func readPair(aRoot, bRoot string, vault, oid ID) ([]byte, error) {
	rawA, err := os.ReadFile(objPath(aRoot, oid))
	if err != nil {
		return nil, fmt.Errorf("share A %s: %w", oid, err)
	}
	rawB, err := os.ReadFile(objPath(bRoot, oid))
	if err != nil {
		return nil, fmt.Errorf("share B %s: %w", oid, err)
	}
	a, err := decodeShare(rawA, SideA, vault, oid)
	if err != nil {
		return nil, err
	}
	b, err := decodeShare(rawB, SideB, vault, oid)
	if err != nil {
		return nil, err
	}
	return split.Join(a, b)
}

// loadIndexFrom loads an index joined from the shares in the given roots.
func loadIndexFrom(aRoot, bRoot string, vault ID, oid string) (*Index, error) {
	id, err := ParseID(oid)
	if err != nil {
		return nil, err
	}
	data, err := readPair(aRoot, bRoot, vault, id)
	if err != nil {
		return nil, err
	}
	defer split.Wipe(data)
	ix := &Index{}
	if err := json.Unmarshal(data, ix); err != nil || ix.Entries == nil || ix.Entries["/"] == nil {
		return nil, fmt.Errorf("%w: index %s", ErrCorrupt, oid)
	}
	if err := ix.validate(); err != nil {
		return nil, fmt.Errorf("%w: index %s: %v", ErrCorrupt, oid, err)
	}
	ix.ensureInos()
	return ix, nil
}

// validate checks that every path is canonical, absolute and has a
// parent directory. Protects export and mounting from paths with ".."
// in a damaged or tampered index (S005, R4-02).
func (ix *Index) validate() error {
	for p := range ix.Entries {
		if p != Clean(p) || !strings.HasPrefix(p, "/") {
			return fmt.Errorf("non-canonical path in the index")
		}
		if p == "/" {
			continue
		}
		parent, ok := ix.Entries[path.Dir(p)]
		if !ok || !parent.Dir {
			return fmt.Errorf("entry without a parent directory")
		}
	}
	return nil
}

// ReadBlock returns the plain content of a block. The caller wipes the buffer.
func (v *Vault) ReadBlock(oid string) ([]byte, error) {
	id, err := ParseID(oid)
	if err != nil {
		return nil, err
	}
	return v.readShares(id)
}

// WriteBlock writes a new object with a new mask and returns its ID.
// The object is not visible until a committed index points to it.
func (v *Vault) WriteBlock(data []byte) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	id, err := v.writeObject(data)
	return id.String(), err
}

func (v *Vault) writeObject(data []byte) (ID, error) {
	if v.broken != nil {
		return ID{}, ErrBroken
	}
	if len(data) > MaxObject {
		return ID{}, errors.New("object too large")
	}
	oid, err := newID()
	if err != nil {
		return ID{}, err
	}
	a, b, err := split.Split(data, v.masks)
	if err != nil {
		return ID{}, v.fail(err)
	}
	if err := v.at("share-A"); err != nil {
		return ID{}, v.fail(err)
	}
	if err := writeAtomic(objPath(v.aRoot, oid), encodeShare(SideA, v.id, oid, a), false); err != nil {
		return ID{}, v.fail(fmt.Errorf("writing share A: %w", err))
	}
	if err := v.at("share-B"); err != nil {
		return ID{}, v.fail(err)
	}
	if err := writeAtomic(objPath(v.bRoot, oid), encodeShare(SideB, v.id, oid, b), false); err != nil {
		return ID{}, v.fail(fmt.Errorf("writing share B: %w", err))
	}
	if v.dirty == nil {
		v.dirty = map[string]bool{}
	}
	v.dirty[filepath.Dir(objPath(v.aRoot, oid))] = true
	v.dirty[filepath.Dir(objPath(v.bRoot, oid))] = true
	split.Wipe(b) // the mask does not stay in memory longer than needed
	return oid, nil
}

// Commit changes the index atomically. mutate works on a copy; an error from mutate does not
// change the state. The commit is durable only after both superblocks are written (FR-10).
func (v *Vault) Commit(mutate func(ix *Index) error) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.broken != nil {
		return ErrBroken
	}
	nix := v.idx.clone()
	if err := mutate(nix); err != nil {
		return err
	}
	return v.commitIndex(nix)
}

func (v *Vault) commitIndex(nix *Index) error {
	data, err := json.Marshal(nix)
	if err != nil {
		return err
	}
	oid, err := v.writeObject(data)
	split.Wipe(data)
	if err != nil {
		return err
	}
	if err := v.at("barrier"); err != nil {
		return v.fail(err)
	}
	for d := range v.dirty {
		syncDir(d, false)
		delete(v.dirty, d)
	}
	for _, r := range []string{v.aRoot, v.bRoot} {
		if err := barrier(r); err != nil {
			return v.fail(fmt.Errorf("syncing %s: %w", r, err))
		}
	}
	newCur := commitRec{Seq: v.cur.Seq + 1, Index: oid.String()}
	if err := v.at("super-A"); err != nil {
		return v.fail(err)
	}
	if err := v.writeSuper(v.aRoot, SideA, newCur, v.cur); err != nil {
		return v.fail(fmt.Errorf("superblock A: %w", err))
	}
	if err := v.at("super-B"); err != nil {
		return v.fail(err)
	}
	if err := v.writeSuper(v.bRoot, SideB, newCur, v.cur); err != nil {
		return v.fail(fmt.Errorf("superblock B: %w", err))
	}
	oldLive, okOld := v.live()
	v.prev, v.prevIdx = v.cur, v.idx
	v.cur, v.idx = newCur, nix
	v.inos = nil
	newLive, okNew := v.live()
	if !okOld || !okNew {
		Logf("xordock: GC paused: cannot determine the objects pinned by snapshots")
		return nil
	}
	for id := range oldLive {
		if !newLive[id] {
			v.removeObject(id)
		}
	}
	return nil
}

// live returns the objects reachable from the current and previous state and from
// all snapshots. ok=false when the index of some snapshot cannot be
// read: then GC must not delete anything (S004 S-04).
func (v *Vault) live() (map[string]bool, bool) {
	m := map[string]bool{}
	if v.cur.Index != "" {
		m[v.cur.Index] = true
	}
	if v.prev.Index != "" {
		m[v.prev.Index] = true
	}
	v.idx.refs(m)
	v.prevIdx.refs(m)
	ok := true
	for _, ix := range []*Index{v.idx, v.prevIdx} {
		if ix == nil {
			continue
		}
		for _, sn := range ix.Snaps {
			refs, err := v.snapLive(sn.Index)
			if err != nil {
				ok = false
				continue
			}
			for k := range refs {
				m[k] = true
			}
		}
	}
	return m, ok
}

// snapLive returns (from the cache) the objects pinned by a snapshot.
// Indexes are immutable, so an entry never goes stale.
func (v *Vault) snapLive(indexOID string) (map[string]bool, error) {
	if r, ok := v.snapRefs[indexOID]; ok {
		return r, nil
	}
	ix, err := v.loadIndex(indexOID)
	if err != nil {
		return nil, err
	}
	r := map[string]bool{indexOID: true}
	ix.refs(r)
	if v.snapRefs == nil {
		v.snapRefs = map[string]map[string]bool{}
	}
	v.snapRefs[indexOID] = r
	return r, nil
}

func (v *Vault) removeObject(hexID string) {
	id, err := ParseID(hexID)
	if err != nil {
		return
	}
	// B first: without the mask the old share A is useless.
	os.Remove(objPath(v.bRoot, id))
	os.Remove(objPath(v.aRoot, id))
}

// gcAll deletes objects unreachable from the current and previous state
// (e.g. leftovers of an interrupted write).
func (v *Vault) gcAll() {
	live, ok := v.live()
	if !ok {
		Logf("xordock: GC skipped: snapshot index unavailable")
		return
	}
	n := 0
	for _, root := range []string{v.bRoot, v.aRoot} {
		filepath.WalkDir(filepath.Join(root, objDir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !live[d.Name()] {
				os.Remove(p)
				n++
			}
			return nil
		})
	}
	if n > 0 {
		Logf("xordock: deleted %d orphaned shares", n)
	}
}

func cleanTmp(root string) {
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".tmp") {
			os.Remove(p)
		}
		return nil
	})
}

// --- index reads ---

func Clean(p string) string {
	p = path.Clean("/" + p)
	return p
}

// Stat returns a copy of an entry.
func (v *Vault) Stat(p string) (Entry, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, ok := v.idx.Entries[Clean(p)]
	if !ok {
		return Entry{}, false
	}
	c := *e
	c.Blocks = append([]string(nil), e.Blocks...)
	return c, true
}

type DirItem struct {
	Name  string
	Entry Entry
}

// List returns the direct children of a directory, sorted by name.
func (v *Vault) List(dir string) []DirItem {
	v.mu.Lock()
	defer v.mu.Unlock()
	dir = Clean(dir)
	var out []DirItem
	for p, e := range v.idx.Entries {
		if p == "/" || path.Dir(p) != dir {
			continue
		}
		out = append(out, DirItem{Name: path.Base(p), Entry: *e})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ByIno returns the path and a copy of the entry with the given number.
func (v *Vault) ByIno(ino uint64) (string, Entry, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.inos == nil {
		v.inos = make(map[uint64]string, len(v.idx.Entries))
		for p, e := range v.idx.Entries {
			v.inos[e.Ino] = p
		}
	}
	p, ok := v.inos[ino]
	if !ok {
		return "", Entry{}, false
	}
	e := *v.idx.Entries[p]
	e.Blocks = append([]string(nil), e.Blocks...)
	return p, e, true
}

// Paths returns all paths in the index (sorted).
func (v *Vault) Paths() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]string, 0, len(v.idx.Entries))
	for p := range v.idx.Entries {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
