package mount

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xordock/internal/vault"
)

func TestMain(m *testing.M) {
	vault.SameDeviceCheck = false
	os.Exit(m.Run())
}

func setup(t *testing.T) (*httptest.Server, string) {
	d := t.TempDir()
	v, err := vault.Create(filepath.Join(d, "A"), filepath.Join(d, "B"), "t", vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewUnstartedServer(nil)
	s.Start()
	host := strings.TrimPrefix(s.URL, "http://")
	s.Config.Handler = Handler(v, "tajnytoken", host)
	t.Cleanup(s.Close)
	return s, s.URL + "/tajnytoken"
}

func status(t *testing.T, req *http.Request) int {
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	return r.StatusCode
}

// Dostęp tylko pod tajną ścieżką, z właściwym Host i bez Origin.
func TestHandlerGuards(t *testing.T) {
	s, base := setup(t)
	put, _ := http.NewRequest("PUT", base+"/plik", bytes.NewReader([]byte("x")))
	if c := status(t, put); c >= 300 {
		t.Fatalf("PUT z tokenem: %d", c)
	}
	for name, req := range map[string]*http.Request{
		"bez tokenu":    must(http.NewRequest("GET", s.URL+"/plik", nil)),
		"zły token":     must(http.NewRequest("GET", s.URL+"/tajnytokenX/plik", nil)),
		"obcy Host":     withHeader(must(http.NewRequest("GET", base+"/plik", nil)), "Host", "evil.example:80"),
		"z Origin":      withHeader(must(http.NewRequest("GET", base+"/plik", nil)), "Origin", "http://evil.example"),
		"prefiks bez /": must(http.NewRequest("GET", s.URL+"/tajnytokenplik", nil)),
	} {
		if c := status(t, req); c != http.StatusForbidden {
			t.Errorf("%s: oczekiwano 403, jest %d", name, c)
		}
	}
}

func must(r *http.Request, err error) *http.Request {
	if err != nil {
		panic(err)
	}
	return r
}

func withHeader(r *http.Request, k, v string) *http.Request {
	if k == "Host" {
		r.Host = v
	} else {
		r.Header.Set(k, v)
	}
	return r
}

// CG-09: błędy logowane przez serwer nie zawierają ścieżek (nazw plików).
func TestStripPaths(t *testing.T) {
	err := &os.PathError{Op: "open", Path: "/TAJNA-NAZWA.txt", Err: os.ErrNotExist}
	if s := stripPaths(err); strings.Contains(s, "TAJNA") {
		t.Fatalf("ścieżka w logu: %q", s)
	}
	if s := stripPaths(errors.New("coś /TAJNA/ścieżka")); strings.Contains(s, "TAJNA") {
		t.Fatalf("ścieżka w logu: %q", s)
	}
}
