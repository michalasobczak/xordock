// Package fusefs tłumaczy operacje systemu plików (FUSE, ścieżki) na API
// silnika sejfu (S004 CP-07). Nie zawiera logiki kryptograficznej: podział,
// maski i commit robi internal/vault.
//
// Zapisy trafiają do buforów w RAM (jawna treść tylko w pamięci, M24) i są
// zatwierdzane jednym commitem przy flush, fsync, release albo odmontowaniu.
// fsync kończy się sukcesem dopiero po utrwaleniu obu udziałów (T-02).
// Przy wielu zmienionych blokach pełne bloki są zapisywane wcześniej jako
// obiekty (bez commitu), żeby duży plik nie wymagał całej treści w RAM;
// przy awarii takie obiekty są sierotami i GC usuwa je przy otwarciu.
package fusefs

import (
	"errors"
	"sort"
	"sync"
	"syscall"
	"time"

	"xordock/internal/split"
	"xordock/internal/vault"
)

const (
	bs = vault.BlockSize
	// maxDirty: liczba zmienionych bloków w RAM na plik, po której pełne
	// bloki zapisujemy jako obiekty (64 MiB przy blokach 1 MiB).
	maxDirty = 64
)

// Attr to atrybuty pliku przekazywane do FUSE.
type Attr struct {
	Ino   uint64
	Dir   bool
	Size  int64
	Mtime time.Time
}

type node struct {
	ino     uint64
	size    int64
	blocks  []string       // "" = dziura albo blok jeszcze niezapisany
	dirty   map[int][]byte // indeks bloku → bufor BlockSize z jawną treścią
	changed bool
	mtime   *time.Time
	refs    int
	cIdx    int // pamięć podręczna jednego czystego bloku
	cache   []byte
}

func (n *node) wipe() {
	for i, b := range n.dirty {
		split.Wipe(b)
		delete(n.dirty, i)
	}
	split.Wipe(n.cache)
	n.cache, n.cIdx = nil, -1
}

// FS to stan zamontowanego sejfu.
type FS struct {
	mu      sync.Mutex
	v       *vault.Vault
	nodes   map[uint64]*node // ino → otwarty plik
	handles map[uint64]*node
	nextFH  uint64
	down    error // karta wyjęta albo sejf w stanie błędu
}

func New(v *vault.Vault) *FS {
	return &FS{v: v, nodes: map[uint64]*node{}, handles: map[uint64]*node{}, nextFH: 1}
}

// Errno mapuje błędy silnika na kody POSIX. Nieznany błąd to EIO, nigdy sukces.
func Errno(err error) syscall.Errno {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, vault.ErrNotExist), errors.Is(err, vault.ErrNoSnapshot):
		return syscall.ENOENT
	case errors.Is(err, vault.ErrExist):
		return syscall.EEXIST
	case errors.Is(err, vault.ErrNotDir):
		return syscall.ENOTDIR
	case errors.Is(err, vault.ErrIsDir):
		return syscall.EISDIR
	case errors.Is(err, vault.ErrNotEmpty):
		return syscall.ENOTEMPTY
	case errors.Is(err, vault.ErrRoot):
		return syscall.EPERM
	}
	var e syscall.Errno
	if errors.As(err, &e) {
		return e
	}
	return syscall.EIO
}

// Unavailable przełącza wolumen w stan niedostępny (F-04): sejf w stanie
// błędu, bufory z jawną treścią wyczyszczone, każda operacja kończy się EIO.
func (f *FS) Unavailable(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down != nil {
		return
	}
	f.down = err
	f.v.MarkBroken(err)
	for _, n := range f.nodes {
		n.wipe()
		n.changed = false
	}
}

func (f *FS) check() syscall.Errno {
	if f.down != nil {
		return syscall.EIO
	}
	if f.v.Broken() != nil {
		return syscall.EIO
	}
	return 0
}

func attrOf(e vault.Entry) Attr {
	return Attr{Ino: e.Ino, Dir: e.Dir, Size: e.Size, Mtime: e.ModTime}
}

func (f *FS) Getattr(path string) (Attr, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down != nil {
		if vault.Clean(path) == "/" {
			return Attr{Ino: 1, Dir: true, Mtime: time.Now()}, 0
		}
		return Attr{}, syscall.EIO
	}
	e, ok := f.v.Stat(path)
	if !ok {
		return Attr{}, syscall.ENOENT
	}
	a := attrOf(e)
	if n, ok := f.nodes[e.Ino]; ok {
		a.Size = n.size
		if n.mtime != nil {
			a.Mtime = *n.mtime
		}
	}
	return a, 0
}

// DirEnt to wpis katalogu.
type DirEnt struct {
	Name string
	Attr Attr
}

func (f *FS) Readdir(path string) ([]DirEnt, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return nil, e
	}
	e, ok := f.v.Stat(path)
	if !ok {
		return nil, syscall.ENOENT
	}
	if !e.Dir {
		return nil, syscall.ENOTDIR
	}
	var out []DirEnt
	for _, it := range f.v.List(path) {
		a := attrOf(it.Entry)
		if n, ok := f.nodes[a.Ino]; ok {
			a.Size = n.size
		}
		out = append(out, DirEnt{Name: it.Name, Attr: a})
	}
	return out, 0
}

func (f *FS) Mkdir(path string) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return e
	}
	return Errno(f.v.Mkdir(path))
}

func (f *FS) Unlink(path string) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return e
	}
	ino, err := f.v.Remove(path, false)
	if err == nil {
		f.orphan(ino)
	}
	return Errno(err)
}

func (f *FS) Rmdir(path string) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return e
	}
	_, err := f.v.Remove(path, true)
	return Errno(err)
}

// orphan: plik usunięty, ale może być otwarty. Jego zmiany już nie trafią
// do indeksu; czyścimy bufory od razu.
func (f *FS) orphan(ino uint64) {
	if n, ok := f.nodes[ino]; ok {
		n.wipe()
		n.changed = false
	}
}

// Flagi rename z libfuse3 (renamex_np na macOS).
const (
	RenameSwap = 1 << 1
	RenameExcl = 1 << 2
)

func (f *FS) Rename(from, to string, flags uint) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return e
	}
	switch flags {
	case 0:
	case RenameExcl:
		if _, ok := f.v.Stat(to); ok {
			return syscall.EEXIST
		}
	default:
		return syscall.ENOTSUP // np. zamiana miejscami: jawny błąd, nie pozorny sukces (F-03)
	}
	replaced, err := f.v.RenameReplacing(from, to)
	if err == nil && replaced != 0 {
		f.orphan(replaced)
	}
	return Errno(err)
}

func (f *FS) open(e vault.Entry) uint64 {
	n, ok := f.nodes[e.Ino]
	if !ok {
		n = &node{ino: e.Ino, size: e.Size, blocks: append([]string(nil), e.Blocks...), dirty: map[int][]byte{}, cIdx: -1}
		f.nodes[e.Ino] = n
	}
	n.refs++
	fh := f.nextFH
	f.nextFH++
	f.handles[fh] = n
	return fh
}

func (f *FS) Open(path string, trunc bool) (uint64, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return 0, e
	}
	e, ok := f.v.Stat(path)
	if !ok {
		return 0, syscall.ENOENT
	}
	if e.Dir {
		return 0, syscall.EISDIR
	}
	fh := f.open(e)
	if trunc {
		f.truncate(f.handles[fh], 0)
	}
	return fh, 0
}

func (f *FS) Create(path string) (uint64, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return 0, e
	}
	if _, err := f.v.Create(path, false); err != nil {
		return 0, Errno(err)
	}
	e, ok := f.v.Stat(path)
	if !ok {
		return 0, syscall.EIO
	}
	return f.open(e), 0
}

func (f *FS) node(fh uint64) (*node, syscall.Errno) {
	if e := f.check(); e != 0 {
		return nil, e
	}
	n, ok := f.handles[fh]
	if !ok {
		return nil, syscall.EBADF
	}
	return n, 0
}

// clean zwraca jawną treść niezmienionego bloku (z pamięci podręcznej).
func (f *FS) clean(n *node, idx int) ([]byte, error) {
	if n.cIdx == idx {
		return n.cache, nil
	}
	if idx >= len(n.blocks) || n.blocks[idx] == "" {
		return nil, nil // dziura: zera
	}
	data, err := f.v.ReadBlock(n.blocks[idx])
	if err != nil {
		return nil, err
	}
	split.Wipe(n.cache)
	n.cache, n.cIdx = data, idx
	return data, nil
}

func (f *FS) Read(fh uint64, off int64, size int) ([]byte, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, e := f.node(fh)
	if e != 0 {
		return nil, e
	}
	if off < 0 {
		return nil, syscall.EINVAL
	}
	if off >= n.size || size <= 0 {
		return nil, 0
	}
	end := min(off+int64(size), n.size)
	out := make([]byte, end-off)
	for pos := off; pos < end; {
		idx := int(pos / bs)
		bo := int(pos % bs)
		chunk := min(int64(bs-bo), end-pos)
		var src []byte
		if d, ok := n.dirty[idx]; ok {
			src = d
		} else {
			var err error
			if src, err = f.clean(n, idx); err != nil {
				split.Wipe(out)
				return nil, Errno(err)
			}
		}
		if bo < len(src) {
			copy(out[pos-off:pos-off+chunk], src[bo:])
		}
		pos += chunk
	}
	return out, 0
}

// dirtyBlock zwraca bufor do modyfikacji bloku idx (wczytuje obecną treść).
func (f *FS) dirtyBlock(n *node, idx int) ([]byte, error) {
	if d, ok := n.dirty[idx]; ok {
		return d, nil
	}
	buf := make([]byte, bs)
	src, err := f.clean(n, idx)
	if err != nil {
		return nil, err
	}
	copy(buf, src)
	if limit := n.size - int64(idx)*bs; limit < bs {
		// poza końcem pliku zawsze zera
		clear(buf[max(limit, 0):])
	}
	n.dirty[idx] = buf
	if n.cIdx == idx {
		split.Wipe(n.cache)
		n.cache, n.cIdx = nil, -1
	}
	return buf, nil
}

func (f *FS) Write(fh uint64, off int64, data []byte) (int, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, e := f.node(fh)
	if e != 0 {
		return 0, e
	}
	if off < 0 {
		return 0, syscall.EINVAL
	}
	end := off + int64(len(data))
	for pos := off; pos < end; {
		idx := int(pos / bs)
		bo := int(pos % bs)
		chunk := min(int64(bs-bo), end-pos)
		buf, err := f.dirtyBlock(n, idx)
		if err != nil {
			return 0, Errno(err)
		}
		copy(buf[bo:], data[pos-off:pos-off+chunk])
		pos += chunk
	}
	if end > n.size {
		n.size = end
	}
	n.changed = true
	n.mtime = nil
	if len(n.dirty) > maxDirty {
		if err := f.spill(n); err != nil {
			return 0, Errno(err)
		}
	}
	return len(data), 0
}

// spill zapisuje pełne zmienione bloki jako obiekty (bez commitu).
func (f *FS) spill(n *node) error {
	full := int(n.size / bs)
	for idx, buf := range n.dirty {
		if idx >= full {
			continue
		}
		id, err := f.v.WriteBlock(buf)
		if err != nil {
			return err
		}
		for len(n.blocks) <= idx {
			n.blocks = append(n.blocks, "")
		}
		n.blocks[idx] = id
		split.Wipe(buf)
		delete(n.dirty, idx)
	}
	return nil
}

func (f *FS) truncate(n *node, size int64) error {
	if size < n.size && size%bs != 0 {
		if _, err := f.dirtyBlock(n, int(size/bs)); err != nil {
			return err
		}
	}
	nb := int((size + bs - 1) / bs)
	for idx, buf := range n.dirty {
		if idx >= nb {
			split.Wipe(buf)
			delete(n.dirty, idx)
		} else if lim := size - int64(idx)*bs; lim < bs {
			clear(buf[lim:])
		}
	}
	if len(n.blocks) > nb {
		n.blocks = n.blocks[:nb]
	}
	if n.cIdx >= nb {
		split.Wipe(n.cache)
		n.cache, n.cIdx = nil, -1
	}
	n.size = size
	n.changed = true
	n.mtime = nil
	return nil
}

// Truncate zmienia rozmiar pliku; fh=0 oznacza plik nieotwarty.
func (f *FS) Truncate(path string, fh uint64, size int64) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return e
	}
	if size < 0 {
		return syscall.EINVAL
	}
	var n *node
	if fh != 0 {
		n = f.handles[fh]
	}
	temp := false
	if n == nil {
		e, ok := f.v.Stat(path)
		if !ok {
			return syscall.ENOENT
		}
		if e.Dir {
			return syscall.EISDIR
		}
		n = f.nodes[e.Ino]
		if n == nil {
			n = f.handles[f.open(e)]
			temp = true
		}
	}
	err := f.truncate(n, size)
	if err == nil && temp {
		err = f.commit(n)
	}
	if temp {
		f.drop(n)
	}
	return Errno(err)
}

// drop zamyka tymczasowy uchwyt utworzony w Truncate.
func (f *FS) drop(n *node) {
	for fh, h := range f.handles {
		if h == n {
			delete(f.handles, fh)
			n.refs--
			break
		}
	}
	if n.refs <= 0 {
		n.wipe()
		delete(f.nodes, n.ino)
	}
}

// commit zatwierdza zmiany pliku jednym commitem (oba udziały utrwalone).
func (f *FS) commit(n *node) error {
	if !n.changed {
		return nil
	}
	nb := int((n.size + bs - 1) / bs)
	blocks := make([]string, nb)
	idxs := make([]int, 0, len(n.dirty))
	for i := range n.dirty {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	var zero []byte
	for i := 0; i < nb; i++ {
		blen := min(int64(bs), n.size-int64(i)*bs)
		switch d, ok := n.dirty[i]; {
		case ok:
			id, err := f.v.WriteBlock(d[:blen])
			if err != nil {
				return err
			}
			blocks[i] = id
		case i < len(n.blocks) && n.blocks[i] != "":
			blocks[i] = n.blocks[i]
		default:
			if zero == nil {
				zero = make([]byte, bs)
			}
			id, err := f.v.WriteBlock(zero[:blen])
			if err != nil {
				return err
			}
			blocks[i] = id
		}
	}
	err := f.v.SetFileIno(n.ino, blocks, n.size, n.mtime)
	if errors.Is(err, vault.ErrNotExist) {
		err = nil // plik usunięty w trakcie: zmiany przepadają razem z nim
	}
	if err != nil {
		return err
	}
	for _, i := range idxs {
		split.Wipe(n.dirty[i])
		delete(n.dirty, i)
	}
	n.blocks = blocks
	n.changed = false
	n.mtime = nil
	return nil
}

// Flush i Fsync zatwierdzają zmiany pliku.
func (f *FS) Flush(fh uint64) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, e := f.node(fh)
	if e != 0 {
		return e
	}
	return Errno(f.commit(n))
}

func (f *FS) Release(fh uint64) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.handles[fh]
	if !ok {
		return syscall.EBADF
	}
	var err error
	if f.check() == 0 {
		err = f.commit(n)
	}
	delete(f.handles, fh)
	n.refs--
	if n.refs <= 0 {
		n.wipe()
		delete(f.nodes, n.ino)
	}
	return Errno(err)
}

// Utimens ustawia czas modyfikacji.
func (f *FS) Utimens(path string, mtime time.Time) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.check(); e != 0 {
		return e
	}
	e, ok := f.v.Stat(path)
	if !ok {
		return syscall.ENOENT
	}
	if n, ok := f.nodes[e.Ino]; ok && n.changed {
		t := mtime
		n.mtime = &t
		return 0
	}
	return Errno(f.v.SetModTime(e.Ino, mtime))
}

// SyncAll zatwierdza wszystkie otwarte pliki (odmontowanie, F-05).
func (f *FS) SyncAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.check() != 0 {
		return vault.ErrBroken
	}
	var first error
	for _, n := range f.nodes {
		if err := f.commit(n); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// BRoot zwraca katalog udziału B (pojemność wolumenu, statfs).
func (f *FS) BRoot() string { return f.v.BRoot() }

// Close czyści bufory po odmontowaniu.
func (f *FS) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		n.wipe()
	}
	f.nodes = map[uint64]*node{}
	f.handles = map[uint64]*node{}
}
