package davfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/webdav"

	"xordock/internal/vault"
)

func TestMain(m *testing.M) {
	vault.SameDeviceCheck = false
	os.Exit(m.Run())
}

func newFS(t *testing.T) (*FS, string, string) {
	d := t.TempDir()
	a, b := filepath.Join(d, "A"), filepath.Join(d, "B")
	v, err := vault.Create(a, b, "t", vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return New(v), a, b
}

func server(t *testing.T, f *FS) *httptest.Server {
	h := &webdav.Handler{FileSystem: f, LockSystem: webdav.NewMemLS()}
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func do(t *testing.T, method, url string, body []byte, hdr ...string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func readBody(t *testing.T, r *http.Response) []byte {
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return b
}

// CG-01 przez protokół WebDAV (ta sama ścieżka co Finder): PUT, GET, Range,
// MKCOL, MOVE, DELETE, PROPFIND.
func TestWebDAVLifecycle(t *testing.T) {
	f, _, _ := newFS(t)
	s := server(t, f)
	data := bytes.Repeat([]byte("0123456789"), 30000) // ~300 KB, kilka bloków
	if r := do(t, "MKCOL", s.URL+"/dok", nil); r.StatusCode != http.StatusCreated {
		t.Fatalf("MKCOL: %d", r.StatusCode)
	}
	if r := do(t, "PUT", s.URL+"/dok/plik.bin", data); r.StatusCode >= 300 {
		t.Fatalf("PUT: %d", r.StatusCode)
	}
	if got := readBody(t, do(t, "GET", s.URL+"/dok/plik.bin", nil)); !bytes.Equal(got, data) {
		t.Fatal("GET zwrócił inne bajty")
	}
	r := do(t, "GET", s.URL+"/dok/plik.bin", nil, "Range", "bytes=65530-65545")
	if got := readBody(t, r); !bytes.Equal(got, data[65530:65546]) {
		t.Fatalf("Range na granicy bloków: %q", got)
	}
	if r := do(t, "MOVE", s.URL+"/dok", nil, "Destination", s.URL+"/nowy"); r.StatusCode >= 300 {
		t.Fatalf("MOVE: %d", r.StatusCode)
	}
	if got := readBody(t, do(t, "GET", s.URL+"/nowy/plik.bin", nil)); !bytes.Equal(got, data) {
		t.Fatal("po MOVE treść się różni")
	}
	pf := readBody(t, do(t, "PROPFIND", s.URL+"/nowy/", nil, "Depth", "1"))
	if !strings.Contains(string(pf), "plik.bin") {
		t.Fatalf("PROPFIND nie pokazuje pliku: %s", pf)
	}
	if r := do(t, "DELETE", s.URL+"/nowy", nil); r.StatusCode >= 300 {
		t.Fatalf("DELETE: %d", r.StatusCode)
	}
	if r := do(t, "GET", s.URL+"/nowy/plik.bin", nil); r.StatusCode != http.StatusNotFound {
		t.Fatalf("po DELETE: %d", r.StatusCode)
	}
}

// Zapisy w dowolnych miejscach (jak edytory i webdavfs przy zapisie
// częściowym), z dziurami i rozszerzaniem, porównane z modelem w pamięci.
func TestRandomWritesMatchModel(t *testing.T) {
	f, _, _ := newFS(t)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(1))
	var model []byte
	for round := 0; round < 30; round++ {
		flag := os.O_RDWR | os.O_CREATE
		if round%10 == 0 {
			flag |= os.O_TRUNC
			model = nil
		}
		h, err := f.OpenFile(ctx, "/plik", flag, 0)
		if err != nil {
			t.Fatal(err)
		}
		for w := 0; w < 3; w++ {
			off := rng.Int63n(int64(len(model)) + 2*bs)
			chunk := make([]byte, rng.Intn(3*bs))
			rng.Read(chunk)
			if _, err := h.Seek(off, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			if _, err := h.Write(chunk); err != nil {
				t.Fatal(err)
			}
			if end := off + int64(len(chunk)); end > int64(len(model)) {
				model = append(model, make([]byte, end-int64(len(model)))...)
			}
			copy(model[off:], chunk)
		}
		if err := h.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := f.V.GetFile("/plik")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, model) {
			t.Fatalf("runda %d: treść różni się od modelu (len %d vs %d)", round, len(got), len(model))
		}
	}
	if r := f.V.Fsck(); !r.OK() {
		t.Fatal(r.Problems)
	}
}

// Czytający widzi poprzednią wersję, dopóki piszący nie zamknie pliku.
func TestWriteVisibleOnlyAfterClose(t *testing.T) {
	f, _, _ := newFS(t)
	ctx := context.Background()
	f.V.PutFile("/x", []byte("stara"))
	h, _ := f.OpenFile(ctx, "/x", os.O_RDWR|os.O_TRUNC, 0)
	h.Write([]byte("nowa treść"))
	if got, _ := f.V.GetFile("/x"); string(got) != "stara" {
		t.Fatalf("przed Close: %q", got)
	}
	h.Close()
	if got, _ := f.V.GetFile("/x"); string(got) != "nowa treść" {
		t.Fatalf("po Close: %q", got)
	}
}

// CG-08: równoległe PUT różnych plików przez HTTP.
func TestConcurrentPUT(t *testing.T) {
	f, _, _ := newFS(t)
	s := server(t, f)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := bytes.Repeat([]byte(fmt.Sprintf("[%02d]", i)), 20000+i*100)
			req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/p%d", s.URL, i), bytes.NewReader(body))
			if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode >= 300 {
				t.Errorf("PUT %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < 12; i++ {
		got, err := f.V.GetFile(fmt.Sprintf("/p%d", i))
		want := bytes.Repeat([]byte(fmt.Sprintf("[%02d]", i)), 20000+i*100)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("plik %d pomieszany: %v", i, err)
		}
	}
}

// FR-18: po awarii zapisu (np. wyjęta karta) kolejne operacje są odrzucane.
func TestBrokenVaultRefusesWrites(t *testing.T) {
	f, _, _ := newFS(t)
	f.V.MarkBroken(fmt.Errorf("karta odłączona"))
	if _, err := f.OpenFile(context.Background(), "/x", os.O_RDWR|os.O_CREATE, 0); err == nil {
		t.Fatal("zapis przyjęty w stanie błędu")
	}
}
