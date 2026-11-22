package vault

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// fullSync syncs a file to the medium. On macOS fsync does not flush the disk
// cache; F_FULLFSYNC does. When the file system does not support it (e.g. some
// FAT drivers), we fall back to a plain fsync.
func fullSync(f *os.File) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_FULLFSYNC, 0); errno == 0 {
		return nil
	}
	return f.Sync()
}

// syncDir syncs a directory entry after rename. The error is ignored because not every
// file system allows fsync on a directory; rename comes after the file fsync anyway.
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

// writeAtomic writes a file via a temporary file, fsync and rename, so after
// a crash the old or the new version remains, never half. full=true forces
// F_FULLFSYNC (flushing the medium cache); for objects we do a single
// barrier() per medium before writing the superblock, which is much faster.
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
	// without full: the commit syncs the directory entry (syncDirs) before the barrier
	return nil
}

// barrier flushes the cache of the medium holding root (F_FULLFSYNC works
// on the whole device). Called before writing the superblock.
func barrier(root string) error {
	f, err := os.Open(root)
	if err != nil {
		return err
	}
	defer f.Close()
	return fullSync(f)
}

// devOf returns the device number holding the path (-1 on error).
func devOf(p string) int64 {
	var st syscall.Stat_t
	if err := syscall.Stat(p, &st); err != nil {
		return -1
	}
	return int64(st.Dev)
}

// lockRoot takes an exclusive vault lock (flock on a file in share A).
// Two processes on one vault are a risk: the GC of one would delete objects
// that the other is just writing.
func lockRoot(root string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("vault is already open in another xordock process")
	}
	return f, nil
}
