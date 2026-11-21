// Package davfs wystawia sejf jako webdav.FileSystem. Jawne dane istnieją
// tylko w pamięci procesu: w buforach otwartych plików, czyszczonych przy
// zamknięciu (FR-13, FR-15).
package davfs

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"time"

	"golang.org/x/net/webdav"

	"xordock/internal/split"
	"xordock/internal/vault"
)

const bs = vault.BlockSize

type FS struct{ V *vault.Vault }

func New(v *vault.Vault) *FS { return &FS{V: v} }

func (f *FS) Mkdir(_ context.Context, name string, _ os.FileMode) error {
	return f.V.Mkdir(name)
}

func (f *FS) RemoveAll(_ context.Context, name string) error {
	return f.V.RemoveAll(name)
}

func (f *FS) Rename(_ context.Context, oldName, newName string) error {
	return f.V.Rename(oldName, newName)
}

func (f *FS) Stat(_ context.Context, name string) (os.FileInfo, error) {
	name = vault.Clean(name)
	e, ok := f.V.Stat(name)
	if !ok {
		return nil, fs.ErrNotExist
	}
	return info(path.Base(name), e.Dir, e.Size, e.ModTime), nil
}

func (f *FS) OpenFile(_ context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	name = vault.Clean(name)
	if err := f.V.Broken(); err != nil {
		return nil, vault.ErrBroken
	}
	e, ok := f.V.Stat(name)
	writable := flag&(os.O_WRONLY|os.O_RDWR) != 0
	h := &file{v: f.V, name: name, writable: writable, cacheIdx: -1}
	switch {
	case !ok && flag&os.O_CREATE == 0:
		return nil, fs.ErrNotExist
	case !ok:
		parent, pok := f.V.Stat(path.Dir(name))
		if !pok {
			return nil, fs.ErrNotExist
		}
		if !parent.Dir {
			return nil, vault.ErrNotDir
		}
		h.created = true
		h.mtime = time.Now()
		return h, nil
	case flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL:
		return nil, fs.ErrExist
	case e.Dir:
		if writable {
			return nil, vault.ErrIsDir
		}
		return &dir{v: f.V, name: name, mtime: e.ModTime}, nil
	}
	h.blocks, h.size, h.oldSize, h.mtime = e.Blocks, e.Size, e.Size, e.ModTime
	if writable && flag&os.O_TRUNC != 0 {
		h.blocks, h.size, h.oldSize, h.created = nil, 0, 0, true
	}
	return h, nil
}

// --- plik ---

type file struct {
	v        *vault.Vault
	name     string
	writable bool
	created  bool // nowy albo obcięty: zatwierdź przy zamknięciu nawet bez zapisu
	blocks   []string
	size     int64
	oldSize  int64
	mtime    time.Time
	pos      int64
	dirty    map[int64][]byte // zmienione bloki, jawne, tylko w RAM
	fresh    map[int64]bool   // bloki zapisane do sejfu przez ten uchwyt
	cacheIdx int64
	cache    []byte
	closed   bool
}

// oldLen to długość bloku idx w zatwierdzonej wersji pliku.
func (h *file) oldLen(idx int64) int64 {
	return max(0, min(bs, h.oldSize-idx*bs))
}

// block zwraca bieżącą treść bloku (z bufora, z sejfu albo zera).
func (h *file) block(idx int64) ([]byte, error) {
	if d, ok := h.dirty[idx]; ok {
		return d, nil
	}
	if idx == h.cacheIdx {
		return h.cache, nil
	}
	if idx >= int64(len(h.blocks)) || h.blocks[idx] == "" {
		return nil, nil // obszar bez bloku: zera
	}
	data, err := h.v.ReadBlock(h.blocks[idx])
	if err != nil {
		return nil, err
	}
	split.Wipe(h.cache)
	h.cacheIdx, h.cache = idx, data
	return data, nil
}

func (h *file) Read(p []byte) (int, error) {
	if h.closed {
		return 0, fs.ErrClosed
	}
	if h.pos >= h.size {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && h.pos < h.size {
		idx, off := h.pos/bs, h.pos%bs
		blk, err := h.block(idx)
		if err != nil {
			return n, err
		}
		limit := min(int64(len(p)-n), bs-off, h.size-h.pos)
		var c int64
		if off < int64(len(blk)) {
			c = int64(copy(p[n:n+int(limit)], blk[off:min(int64(len(blk)), off+limit)]))
		}
		for i := c; i < limit; i++ { // dziura albo blok krótszy niż rozmiar: zera
			p[n+int(i)] = 0
		}
		n += int(limit)
		h.pos += limit
	}
	return n, nil
}

func (h *file) Write(p []byte) (int, error) {
	if h.closed {
		return 0, fs.ErrClosed
	}
	if !h.writable {
		return 0, fs.ErrPermission
	}
	if h.dirty == nil {
		h.dirty = map[int64][]byte{}
	}
	n := 0
	for n < len(p) {
		idx, off := h.pos/bs, h.pos%bs
		buf, ok := h.dirty[idx]
		if !ok {
			cur, err := h.block(idx)
			if err != nil {
				return n, err
			}
			buf = append(make([]byte, 0, bs), cur...)
			if idx == h.cacheIdx {
				split.Wipe(h.cache)
				h.cacheIdx, h.cache = -1, nil
			}
		}
		c := min(int64(len(p)-n), bs-off)
		if need := off + c; int64(len(buf)) < need {
			buf = append(buf, make([]byte, need-int64(len(buf)))...)
		}
		copy(buf[off:off+c], p[n:n+int(c)])
		h.dirty[idx] = buf
		n += int(c)
		h.pos += c
		h.size = max(h.size, h.pos)
	}
	return n, h.spill()
}

// spill zapisuje do sejfu pełne bloki, których zapis sekwencyjny już minął,
// żeby duży plik nie trzymał całej jawnej treści w RAM. Obiekty stają się
// częścią pliku dopiero przy commicie w Close.
func (h *file) spill() error {
	cur := h.pos / bs
	for idx, buf := range h.dirty {
		if idx >= cur || int64(len(buf)) != bs {
			continue
		}
		if err := h.store(idx, buf); err != nil {
			return err
		}
	}
	return nil
}

func (h *file) store(idx int64, buf []byte) error {
	id, err := h.v.WriteBlock(buf)
	if err != nil {
		return err
	}
	for int64(len(h.blocks)) <= idx {
		h.blocks = append(h.blocks, "")
	}
	h.blocks[idx] = id
	if h.fresh == nil {
		h.fresh = map[int64]bool{}
	}
	h.fresh[idx] = true
	split.Wipe(buf)
	delete(h.dirty, idx)
	h.created = true
	return nil
}

func (h *file) Seek(offset int64, whence int) (int64, error) {
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = h.pos
	case io.SeekEnd:
		base = h.size
	default:
		return 0, fs.ErrInvalid
	}
	if base+offset < 0 {
		return 0, fs.ErrInvalid
	}
	h.pos = base + offset
	return h.pos, nil
}

// Close zatwierdza zmiany jednym commitem. Do tego momentu poprzednia
// wersja pliku pozostaje obowiązująca.
func (h *file) Close() error {
	if h.closed {
		return nil
	}
	h.closed = true
	defer h.wipe()
	if !h.created && len(h.dirty) == 0 {
		return nil
	}
	n := (h.size + bs - 1) / bs
	for idx := int64(0); idx < n; idx++ {
		want := min(bs, h.size-idx*bs)
		buf, ok := h.dirty[idx]
		if !ok {
			have := idx < int64(len(h.blocks)) && h.blocks[idx] != ""
			if have && (h.fresh[idx] || h.oldLen(idx) == want) {
				continue
			}
			// blok dziury albo dawny ostatni, krótszy blok: dopełnij zerami
			cur, err := h.block(idx)
			if err != nil {
				return err
			}
			buf = append(make([]byte, 0, bs), cur...)
		}
		if int64(len(buf)) < want {
			buf = append(buf, make([]byte, want-int64(len(buf)))...)
		}
		if err := h.store(idx, buf[:want]); err != nil {
			return err
		}
	}
	blocks := append([]string(nil), h.blocks[:min(int64(len(h.blocks)), n)]...)
	return h.v.SetFile(h.name, blocks, h.size)
}

func (h *file) wipe() {
	for k, b := range h.dirty {
		split.Wipe(b)
		delete(h.dirty, k)
	}
	split.Wipe(h.cache)
	h.cache, h.cacheIdx = nil, -1
}

func (h *file) Readdir(int) ([]fs.FileInfo, error) { return nil, vault.ErrNotDir }

func (h *file) Stat() (fs.FileInfo, error) {
	return info(path.Base(h.name), false, h.size, h.mtime), nil
}

// --- katalog ---

type dir struct {
	v     *vault.Vault
	name  string
	mtime time.Time
	items []vault.DirItem
	read  bool
}

func (d *dir) Readdir(count int) ([]fs.FileInfo, error) {
	if !d.read {
		d.items, d.read = d.v.List(d.name), true
	}
	if count > 0 && len(d.items) == 0 {
		return nil, io.EOF
	}
	take := len(d.items)
	if count > 0 && count < take {
		take = count
	}
	out := make([]fs.FileInfo, 0, take)
	for _, it := range d.items[:take] {
		out = append(out, info(it.Name, it.Entry.Dir, it.Entry.Size, it.Entry.ModTime))
	}
	d.items = d.items[take:]
	return out, nil
}

func (d *dir) Stat() (fs.FileInfo, error)     { return info(path.Base(d.name), true, 0, d.mtime), nil }
func (d *dir) Read([]byte) (int, error)       { return 0, vault.ErrIsDir }
func (d *dir) Write([]byte) (int, error)      { return 0, vault.ErrIsDir }
func (d *dir) Seek(int64, int) (int64, error) { return 0, nil }
func (d *dir) Close() error                   { return nil }

// --- FileInfo ---

type fi struct {
	name  string
	dir   bool
	size  int64
	mtime time.Time
}

func info(name string, dir bool, size int64, mtime time.Time) fs.FileInfo {
	if name == "/" || name == "." {
		name = "/"
	}
	return fi{name, dir, size, mtime}
}

func (i fi) Name() string       { return i.name }
func (i fi) Size() int64        { return i.size }
func (i fi) ModTime() time.Time { return i.mtime }
func (i fi) IsDir() bool        { return i.dir }
func (i fi) Sys() any           { return nil }
func (i fi) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o700
	}
	return 0o600
}

var _ webdav.FileSystem = (*FS)(nil)
