package vault

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPerf mierzy przepustowość na prawdziwych nośnikach (S001 §8, efekt
// mierzalny). Uruchamiany ręcznie:
//
//	XORDOCK_PERF_A=/ścieżka/na/SSD XORDOCK_PERF_B=/Volumes/karta/dir go test -run TestPerf -v ./internal/vault
func TestPerf(t *testing.T) {
	aDir, bDir := os.Getenv("XORDOCK_PERF_A"), os.Getenv("XORDOCK_PERF_B")
	if aDir == "" || bDir == "" {
		t.Skip("ustaw XORDOCK_PERF_A i XORDOCK_PERF_B")
	}
	a, b := filepath.Join(aDir, "perf-A"), filepath.Join(bDir, "perf-B")
	os.RemoveAll(a)
	os.RemoveAll(b)
	defer os.RemoveAll(a)
	defer os.RemoveAll(b)
	v, err := Create(a, b, "perf", Options{})
	if err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 20<<20)
	rand.Read(big)
	t0 := time.Now()
	if err := v.PutFile("/big", big); err != nil {
		t.Fatal(err)
	}
	w := time.Since(t0)
	t0 = time.Now()
	if _, err := v.GetFile("/big"); err != nil {
		t.Fatal(err)
	}
	r := time.Since(t0)
	t0 = time.Now()
	for i := 0; i < 50; i++ {
		if err := v.PutFile("/small", []byte("mały plik")); err != nil {
			t.Fatal(err)
		}
	}
	s := time.Since(t0) / 50
	t.Logf("zapis 20 MiB: %v (%.1f MiB/s), odczyt: %v (%.1f MiB/s), mały plik: %v",
		w, 20/w.Seconds(), r, 20/r.Seconds(), s)
}
