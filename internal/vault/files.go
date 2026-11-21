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
		if e, ok := ix.Entries[p]; ok && e.Dir {
			return ErrIsDir
		}
		ix.Entries[p] = &Entry{Size: size, ModTime: time.Now().UTC(), Blocks: blocks}
		ix.Entries[path.Dir(p)].ModTime = time.Now().UTC()
		return nil
	})
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
		ix.Entries[p] = &Entry{Dir: true, ModTime: time.Now().UTC()}
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
	from, to = Clean(from), Clean(to)
	if from == "/" || to == "/" {
		return ErrRoot
	}
	if from == to {
		return nil
	}
	return v.Commit(func(ix *Index) error {
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
		return nil
	})
}
