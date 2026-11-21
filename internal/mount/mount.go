// Package mount udostępnia sejf w Finderze: lokalny serwer WebDAV na
// 127.0.0.1 i wbudowany w macOS mount_webdav. Pilnuje karty: gdy zniknie,
// sejf przechodzi w stan błędu, a katalog jest odmontowywany (SC-02, FR-18).
package mount

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/webdav"

	"xordock/internal/davfs"
	"xordock/internal/vault"
)

// Handler zwraca handler HTTP sejfu pod tajną ścieżką token. Odrzuca żądania
// spoza ścieżki, z obcym Host i z nagłówkiem Origin (przeglądarki), co
// utrudnia dostęp innym procesom i atakom DNS rebinding.
func Handler(v *vault.Vault, token, host string) http.Handler {
	prefix := "/" + token
	dav := &webdav.Handler{
		Prefix:     prefix,
		FileSystem: davfs.New(v),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				// bez ścieżki: nazwy plików są treścią chronioną (FR-15)
				vault.Logf("xordock: %s: %v", r.Method, stripPaths(err))
			}
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host || r.Header.Get("Origin") != "" ||
			(r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/")) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		dav.ServeHTTP(w, r)
	})
}

// stripPaths usuwa z błędu ścieżki (os.PathError i podobne).
func stripPaths(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Op + ": " + pe.Err.Error()
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Op + ": " + le.Err.Error()
	}
	s := err.Error()
	if strings.Contains(s, "/") {
		return "błąd operacji (szczegóły pominięte)"
	}
	return s
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IsMounted sprawdza, czy pod mountpoint jest zamontowany WebDAV.
func IsMounted(mp string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(mp, &st); err != nil {
		return false
	}
	var b strings.Builder
	for _, c := range st.Fstypename {
		if c == 0 {
			break
		}
		b.WriteByte(byte(c))
	}
	return b.String() == "webdav"
}

func unmount(mp string, force bool) error {
	args := []string{mp}
	if force {
		args = []string{"-f", mp}
	}
	out, err := exec.Command("/sbin/umount", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Run montuje sejf i blokuje do odmontowania, sygnału albo utraty karty.
func Run(v *vault.Vault, mountpoint string, status func(string)) error {
	if IsMounted(mountpoint) {
		return fmt.Errorf("%s jest już zamontowany", mountpoint)
	}
	if err := os.MkdirAll(mountpoint, 0o700); err != nil {
		return err
	}
	token, err := randomToken()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	host := ln.Addr().String()
	srv := &http.Server{Handler: Handler(v, token, host), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	url := fmt.Sprintf("http://%s/%s/", host, token)
	out, err := exec.Command("/sbin/mount_webdav", "-S", "-v", v.Name(), url, mountpoint).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount_webdav: %v: %s", err, strings.TrimSpace(string(out)))
	}
	status(fmt.Sprintf("zamontowano %s", mountpoint))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	cardMarker := filepath.Join(v.BRoot(), "xordock.json")
	for {
		select {
		case <-sig:
			status("odmontowywanie…")
			// FR-19: zwykły umount czeka na zakończenie operacji; Close plików
			// zatwierdza zmiany, zanim serwer zostanie zatrzymany.
			if err := unmount(mountpoint, false); err != nil {
				return fmt.Errorf("%w (zamknij pliki w katalogu i spróbuj ponownie)", err)
			}
			return nil
		case <-tick.C:
			if _, err := os.Stat(cardMarker); err != nil {
				v.MarkBroken(errors.New("karta SD odłączona"))
				unmount(mountpoint, true)
				return errors.New("karta SD odłączona: sejf odmontowany, niezatwierdzone zmiany odrzucone")
			}
			if !IsMounted(mountpoint) {
				status("katalog odmontowany")
				return nil
			}
		}
	}
}
