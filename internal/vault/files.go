package vault

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"time"

	"xordock/internal/split"
)

// Operacje na ścieżkach. Każda kończy się jednym commitem, więc jest atomowa
// względem awarii (FR-22). Używane przez warstwę WebDAV, eksport i testy.

var (
	ErrNotExist = fs.ErrNotExist
	ErrExist    = fs.ErrExist
	ErrNotDir   = errors.New("nie jest katalogiem")
	ErrIsDir    = errors.New("jest katalogiem")
	ErrNotEmpty = errors.New("katalog nie jest pusty")
	ErrRoot     = errors.New("operacja niedozwolona na katalogu głównym")
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

// WriteBlocks dzieli dane na bloki, każdy z nową maską, i zwraca ich identyfikatory.
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

// SetFile zatwierdza nową zawartość pliku (lista bloków i rozmiar).
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

// SetFileIno zatwierdza nową zawartość pliku wskazanego numerem (ścieżka
// mogła się zmienić od otwarcia). mtime nil oznacza „teraz”.
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

// SetModTime ustawia czas modyfikacji pliku albo katalogu.
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

// Create tworzy pusty plik. exclusive=true: błąd, gdy istnieje. Zwraca numer pliku.
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

// errNoChange przerywa commit bez błędu dla wywołującego.
var errNoChange = errors.New("bez zmian")

// Remove usuwa plik albo pusty katalog (semantyka unlink/rmdir).
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

// PutFile zapisuje cały plik.
func (v *Vault) PutFile(p string, data []byte) error {
	blocks, err := v.WriteBlocks(data)
	if err != nil {
		return err
	}
	return v.SetFile(p, blocks, int64(len(data)))
}

// GetFile odczytuje cały plik. Wywołujący czyści wynik split.Wipe.
func (v *Vault) GetFile(p string) ([]byte, error) {
	e, ok := v.Stat(p)
	if !ok {
		return nil, ErrNotExist
	}
	if e.Dir {
		return nil, ErrIsDir
	}
	out := make([]byte, 0, e.Size)
	for _, b := range e.Blocks {
		blk, err := v.ReadBlock(b)
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

// RemoveAll usuwa plik albo katalog z zawartością.
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

// Rename przenosi plik albo katalog. Bloki nie są przepisywane; zmienia się
// tylko indeks. Istniejący plik docelowy jest zastępowany, katalog nie.
func (v *Vault) Rename(from, to string) error {
	_, err := v.RenameReplacing(from, to)
	return err
}

// RenameReplacing działa jak Rename i zwraca numer zastąpionego pliku (0, gdy nie było).
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
			return errors.New("nie można przenieść katalogu do jego wnętrza")
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
