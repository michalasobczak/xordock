package fusemount

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"xordock/internal/fusefs"
	"xordock/internal/vault"
)

// ErrNoMacFUSE: the libfuse3 library from macFUSE is missing (S011).
var ErrNoMacFUSE = errors.New("macFUSE 5.5+ is missing (library /usr/local/lib/libfuse3.4.dylib); install macFUSE from https://macfuse.github.io/")

var (
	errFuseNew = errors.New("libfuse rejected the mount options")
	errMount   = errors.New("mount failed: check that the macFUSE extension is enabled in System Settings → General → Login Items & Extensions → File System Extensions")
)

// Mount exposes the vault in Finder and blocks until unmounted. onMounted
// is called only once the volume is actually mounted. Every second it
// checks that the working card is present; once the card is removed the volume
// becomes unavailable and is force-unmounted (F-04, SC-03).
func Mount(v *vault.Vault, mountpoint, volname string, onMounted func()) error {
	if !Available() {
		return ErrNoMacFUSE
	}
	fs := fusefs.New(v)
	defer fs.Close()
	done := make(chan struct{})
	defer close(done)
	var mounted atomic.Bool
	go func() {
		for i := 0; i < 300 && !mounted.Load(); i++ {
			select {
			case <-done:
				return
			case <-time.After(100 * time.Millisecond):
			}
			if IsMounted(mountpoint) {
				mounted.Store(true)
				onMounted()
			}
		}
	}()

	go watchCard(fs, v, mountpoint, done)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			if err := fs.SyncAll(); err != nil {
				vault.Logf("xordock: commit before unmount: %v", err)
			}
			unmount(mountpoint, false)
		case <-done:
		}
	}()
	opts := fmt.Sprintf("backend=fskit,volname=%s,fsname=xordock", volname)
	err := run(fs, mountpoint, opts)
	if err == nil && !mounted.Load() && !IsMounted(mountpoint) {
		// libfuse can end the loop with code 0 when mounting failed.
		return errMount
	}
	return err
}

// IsMounted: the mount point is on a different device than its parent directory.
func IsMounted(mp string) bool {
	var a, b syscall.Stat_t
	if syscall.Stat(mp, &a) != nil || syscall.Stat(filepath.Dir(mp), &b) != nil {
		return false
	}
	return a.Dev != b.Dev
}

func watchCard(fs *fusefs.FS, v *vault.Vault, mountpoint string, done <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	header := filepath.Join(v.BRoot(), "xordock.json")
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if _, err := os.Stat(header); err != nil {
				vault.Logf("xordock: working card unavailable, volume locked and being unmounted")
				fs.Unavailable(fmt.Errorf("working card removed: %w", err))
				unmount(mountpoint, true)
				return
			}
		}
	}
}

// unmount unmounts the volume; force=true when the card is lost.
func unmount(mountpoint string, force bool) {
	args := []string{"unmount"}
	if force {
		args = append(args, "force")
	}
	args = append(args, mountpoint)
	if out, err := exec.Command("/usr/sbin/diskutil", args...).CombinedOutput(); err != nil {
		vault.Logf("xordock: diskutil unmount: %v: %s", err, out)
		if err := exec.Command("/sbin/umount", "-f", mountpoint).Run(); err != nil {
			exitLoop()
		}
	}
}

// Unmount unmounts the volume (the xordock unmount command).
func Unmount(mountpoint string) error {
	out, err := exec.Command("/usr/sbin/diskutil", "unmount", mountpoint).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}
