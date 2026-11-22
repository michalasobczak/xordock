package vault

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPerf measures throughput on real media (S001 §8, measurable
// effect). Run manually:
//
//	XORDOCK_PERF_A=/path/on/SSD XORDOCK_PERF_B=/Volumes/card/dir go test -run TestPerf -v ./internal/vault
func TestPerf(t *testing.T) {
	aDir, bDir := os.Getenv("XORDOCK_PERF_A"), os.Getenv("XORDOCK_PERF_B")
	if aDir == "" || bDir == "" {
		t.Skip("set XORDOCK_PERF_A and XORDOCK_PERF_B")
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
		if err := v.PutFile("/small", []byte("small file")); err != nil {
			t.Fatal(err)
		}
	}
	s := time.Since(t0) / 50
	t.Logf("write 20 MiB: %v (%.1f MiB/s), read: %v (%.1f MiB/s), small file: %v",
		w, 20/w.Seconds(), r, 20/r.Seconds(), s)
}
