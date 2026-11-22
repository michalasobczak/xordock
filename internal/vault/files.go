package vault

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"time"
)

// Path operations. Each ends with a single commit, so it is atomic
// with respect to crashes (FR-22). Used by the FUSE adapter, export and tests.

var (
	ErrNotExist = fs.ErrNotExist
	ErrExist    = fs.ErrExist
	ErrNotDir   = errors.New("not a directory")
	ErrIsDir    = errors.New("is a directory")
	ErrNotEmpty = errors.New("directory not empty")
	ErrRoot     = errors.New("operation not allowed on the root directory")
)

func parentDir(ix *Index, p string) error {
	parent, ok := ix.Entries[path.Dir(p)]
	if !ok {
		return ErrNotExist
	}
	if !parent.Dir {
		return ErrNotDir
	}
	return nil
}

// WriteBlocks splits data into blocks, each with a new mask, and returns their IDs.
func (v *Vault) WriteBlocks(data []byte) ([]string, error) {
	var ids []string
	for off := 0; off < len(data); off += BlockSize {
		end := min(off+BlockSize, len(data))
		id, err := v.WriteBlock(data[off:end])
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// SetFile commits new file content (block list and size).
func (v *Vault) SetFile(p string, blocks []string, size int64) error {
	p = Clean(p)
	if p == "/" {
		return ErrRoot
	}
	return v.Commit(func(ix *Index) error {
		if err := parentDir(ix, p); err != nil {
			return err
		}
		ino := uint64(0)
		if e, ok := ix.Entries[p]; ok {
			if e.Dir {
				return ErrIsDir
			}
			ino = e.Ino
		} else {
			ino = ix.allocIno()
		}
		ix.Entries[p] = &Entry{Ino: ino, Size: size, ModTime: time.Now().UTC(), Blocks: blocks}
		ix.Entries[path.Dir(p)].ModTime = time.Now().UTC()
		return nil
	})
}

// SetFileIno commits new content of the file given by number (the path
// may have changed since opening). mtime nil means "now".
func (v *Vault) SetFileIno(ino uint64, blocks []string, size int64, mtime *time.Time) error {
	return v.Commit(func(ix *Index) error {
		p, ok := ix.pathOf(ino)
		if !ok {
			return ErrNotExist
		}
		e := ix.Entries[p]
		if e.Dir {
			return ErrIsDir
		}
		t := time.Now().UTC()
		if mtime != nil {
			t = mtime.UTC()
		}
		ix.Entries[p] = &Entry{Ino: ino, Size: size, ModTime: t, Blocks: blocks}
		return nil
	})
}

// SetModTime sets the modification time of a file or directory.
func (v *Vault) SetModTime(ino uint64, mtime time.Time) error {
	return v.Commit(func(ix *Index) error {
		p, ok := ix.pathOf(ino)
		if !ok {
			return ErrNotExist
		}
		ix.Entries[p].ModTime = mtime.UTC()
		return nil
	})
}

// Create creates an empty file. exclusive=true: error if it exists. Returns the file number.
func (v *Vault) Create(p string, exclusive bool) (uint64, error) {
	p = Clean(p)
	var ino uint64
	err := v.Commit(func(ix *Index) error {
		if err := parentDir(ix, p); err != nil {
			return err
		}
		if e, ok := ix.Entries[p]; ok {
			if exclusive {
				return ErrExist
			}
			if e.Dir {
				return ErrIsDir
			}
			ino = e.Ino
			return errNoChange
		}
		ino = ix.allocIno()
		ix.Entries[p] = &Entry{Ino: ino, ModTime: time.Now().UTC()}
		ix.Entries[path.Dir(p)].ModTime = time.Now().UTC()
		return nil
	})
	if errors.Is(err, errNoChange) {
		err = nil
	}
	return ino, err
}

// errNoChange aborts the commit without an error for the caller.
var errNoChange = errors.New("no changes")

// Remove deletes a file or an empty directory (unlink/rmdir semantics).
func (v *Vault) Remove(p string, dir bool) (uint64, error) {
	p = Clean(p)
	if p == "/" {
		return 0, ErrRoot
	}
	var ino uint64
	err := v.Commit(func(ix *Index) error {
		e, ok := ix.Entries[p]
		if !ok {
			return ErrNotExist
		}
		if e.Dir != dir {
			if e.Dir {
				return ErrIsDir
			}
			return ErrNotDir
		}
		if dir {
			for k := range ix.Entries {
				if k != p && under(k, p) {
					return ErrNotEmpty
				}
			}
		}
		ino = e.Ino
		delete(ix.Entries, p)
		ix.Entries[path.Dir(p)].ModTime = time.Now().UTC()
		return nil
	})
	return ino, err
}

// PutFile writes a whole file.
func (v *Vault) PutFile(p string, data []byte) error {
	blocks, err := v.WriteBlocks(data)
	if err != nil {
		return err
	}
	return v.SetFile(p, blocks, int64(len(data)))
}

// GetFile reads a whole file. The caller wipes the result with split.Wipe.
func (v *Vault) GetFile(p string) ([]byte, error) {
	e, ok := v.Stat(p)
	if !ok {
		return nil, ErrNotExist
	}
	return readEntry(e, v.ReadBlock)
}

func (v *Vault) Mkdir(p string) error {
	p = Clean(p)
	return v.Commit(func(ix *Index) error {
		if _, ok := ix.Entries[p]; ok {
			return ErrExist
		}
		if err := parentDir(ix, p); err != nil {
			return err
		}
		ix.Entries[p] = &Entry{Ino: ix.allocIno(), Dir: true, ModTime: time.Now().UTC()}
		ix.Entries[path.Dir(p)].ModTime = time.Now().UTC()
		return nil
	})
}

func under(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// RemoveAll deletes a file or a directory with its content.
func (v *Vault) RemoveAll(p string) error {
	p = Clean(p)
	if p == "/" {
		return ErrRoot
	}
	return v.Commit(func(ix *Index) error {
		if _, ok := ix.Entries[p]; !ok {
			return ErrNotExist
		}
		for k := range ix.Entries {
			if under(k, p) {
				delete(ix.Entries, k)
			}
		}
		return nil
	})
}

// Rename moves a file or directory. Blocks are not rewritten; only the
// index changes. An existing target file is replaced, a directory is not.
func (v *Vault) Rename(from, to string) error {
	_, err := v.RenameReplacing(from, to)
	return err
}

// RenameReplacing works like Rename and returns the number of the replaced file (0 if none).
func (v *Vault) RenameReplacing(from, to string) (uint64, error) {
	from, to = Clean(from), Clean(to)
	if from == "/" || to == "/" {
		return 0, ErrRoot
	}
	if from == to {
		return 0, nil
	}
	var replaced uint64
	err := v.Commit(func(ix *Index) error {
		src, ok := ix.Entries[from]
		if !ok {
			return ErrNotExist
		}
		if src.Dir && under(to, from) {
			return errors.New("cannot move a directory into itself")
		}
		if err := parentDir(ix, to); err != nil {
			return err
		}
		if dst, ok := ix.Entries[to]; ok {
			if dst.Dir {
				for k := range ix.Entries {
					if k != to && under(k, to) {
						return ErrNotEmpty
					}
				}
			} else if src.Dir {
				return ErrNotDir
			}
			replaced = dst.Ino
			delete(ix.Entries, to)
		}
		moved := map[string]*Entry{}
		for k, e := range ix.Entries {
			if under(k, from) {
				moved[to+strings.TrimPrefix(k, from)] = e
				delete(ix.Entries, k)
			}
		}
		for k, e := range moved {
			ix.Entries[k] = e
		}
		now := time.Now().UTC()
		ix.Entries[path.Dir(from)].ModTime = now
		ix.Entries[path.Dir(to)].ModTime = now
		return nil
	})
	return replaced, err
}
