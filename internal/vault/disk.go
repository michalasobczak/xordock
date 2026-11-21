package vault

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// fullSync utrwala plik na nośniku. Na macOS fsync nie opróżnia cache
// dysku; F_FULLFSYNC to robi. Gdy system plików go nie obsługuje (np. część
// sterowników FAT), wracamy do zwykłego fsync.
func fullSync(f *os.File) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_FULLFSYNC, 0); errno == 0 {
		return nil
	}
	return f.Sync()
}

// syncDir utrwala wpis katalogu po rename. Błąd ignorujemy, bo nie każdy
// system plików pozwala na fsync katalogu; rename i tak jest po fsync pliku.
func syncDir(dir string, full bool) {
	if d, err := os.Open(dir); err == nil {
		if full {
			_ = fullSync(d)
		} else {
			_ = d.Sync()
		}
		d.Close()
	}
}

// writeAtomic zapisuje plik przez plik tymczasowy, fsync i rename, więc po
// awarii zostaje stara albo nowa wersja, nigdy połowa. full=true wymusza
// F_FULLFSYNC (opróżnienie cache nośnika); dla obiektów robimy jeden
// barrier() na nośnik przed zapisem superbloku, co jest znacznie szybsze.
func writeAtomic(path string, data []byte, full bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	sync := f.Sync
	if full {
		sync = func() error { return fullSync(f) }
	}
	if err := sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	if full {
		syncDir(dir, true)
	}
	// bez full: wpis katalogu utrwala commit (syncDirs) przed barierą
	return nil
}

// barrier opróżnia cache nośnika, na którym leży root (F_FULLFSYNC działa
// na całe urządzenie). Wywoływany przed zapisem superbloku.
func barrier(root string) error {
	f, err := os.Open(root)
	if err != nil {
		return err
	}
	defer f.Close()
	return fullSync(f)
}

// devOf zwraca numer urządzenia, na którym leży ścieżka (-1 przy błędzie).
func devOf(p string) int64 {
	var st syscall.Stat_t
	if err := syscall.Stat(p, &st); err != nil {
		return -1
	}
	return int64(st.Dev)
}

// lockRoot zakłada wyłączną blokadę sejfu (flock na pliku w udziale A).
// Dwa procesy na jednym sejfie to ryzyko: GC jednego usunąłby obiekty,
// które drugi właśnie zapisuje.
func lockRoot(root string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("sejf jest już otwarty przez inny proces xordock")
	}
	return f, nil
}
