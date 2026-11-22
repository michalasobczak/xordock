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

// ErrNoMacFUSE: brak biblioteki libfuse3 z macFUSE (S011).
var ErrNoMacFUSE = errors.New("brak macFUSE 5.5+ (biblioteka /usr/local/lib/libfuse3.4.dylib); zainstaluj macFUSE ze strony https://macfuse.github.io/")

var (
	errFuseNew = errors.New("libfuse odrzucił opcje montowania")
	errMount   = errors.New("montowanie nieudane: sprawdź, czy rozszerzenie macFUSE jest włączone w Ustawieniach systemowych → Ogólne → Rozszerzenia i elementy logowania → Rozszerzenia systemu plików")
)

// Mount udostępnia sejf w Finderze i blokuje do odmontowania. onMounted
// jest wołane dopiero, gdy wolumen faktycznie jest zamontowany. Co sekundę
// sprawdza obecność karty roboczej; po jej wyjęciu wolumen przechodzi w stan
// niedostępny i jest wymuszenie odmontowywany (F-04, SC-03).
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
				vault.Logf("xordock: zatwierdzenie przed odmontowaniem: %v", err)
			}
			unmount(mountpoint, false)
		case <-done:
		}
	}()
	opts := fmt.Sprintf("backend=fskit,volname=%s,fsname=xordock", volname)
	err := run(fs, mountpoint, opts)
	if err == nil && !mounted.Load() && !IsMounted(mountpoint) {
		// libfuse potrafi zakończyć pętlę z kodem 0, gdy montowanie się nie udało.
		return errMount
	}
	return err
}

// IsMounted: punkt montowania leży na innym urządzeniu niż jego katalog nadrzędny.
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
				vault.Logf("xordock: karta robocza niedostępna, wolumen zablokowany i odmontowywany")
				fs.Unavailable(fmt.Errorf("karta robocza wyjęta: %w", err))
				unmount(mountpoint, true)
				return
			}
		}
	}
}

// unmount odmontowuje wolumen; force=true przy utracie karty.
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

// Unmount odmontowuje wolumen (polecenie xordock unmount).
func Unmount(mountpoint string) error {
	out, err := exec.Command("/usr/sbin/diskutil", "unmount", mountpoint).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}
