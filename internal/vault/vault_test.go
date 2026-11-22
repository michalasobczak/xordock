package vault

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"xordock/internal/split"
)

func TestMain(m *testing.M) {
	SameDeviceCheck = false // in tests A and B are in one temporary directory
	os.Exit(m.Run())
}

func roots(t *testing.T) (string, string) {
	d := t.TempDir()
	return filepath.Join(d, "A"), filepath.Join(d, "B")
}

func mustCreate(t *testing.T, a, b string, opt Options) *Vault {
	t.Helper()
	v, err := Create(a, b, "test", opt)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustOpen(t *testing.T, a, b string) *Vault {
	t.Helper()
	v, err := Open(a, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func content(n int, seed string) []byte {
	return bytes.Repeat([]byte(seed), n/len(seed)+1)[:n]
}

// CG-01: reading after a write and after reopening gives identical bytes.
func TestRoundTripAfterReopen(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	sizes := []int{0, 1, BlockSize - 1, BlockSize, BlockSize + 1, 3*BlockSize + 5}
	for _, n := range sizes {
		if err := v.PutFile(fmt.Sprintf("/f%d", n), content(n, "abcdefg")); err != nil {
			t.Fatal(err)
		}
	}
	v2 := mustOpen(t, a, b)
	for _, n := range sizes {
		got, err := v2.GetFile(fmt.Sprintf("/f%d", n))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, content(n, "abcdefg")) {
			t.Fatalf("size %d: content differs", n)
		}
	}
	if r := v2.Fsck(); !r.OK() {
		t.Fatal(r.Problems)
	}
}

func scanFor(t *testing.T, root string, needles ...[]byte) {
	t.Helper()
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(p)
		for _, n := range needles {
			if bytes.Contains(data, n) || strings.Contains(p, string(n)) {
				t.Errorf("%s contains plain content %q", p, n)
			}
		}
		return nil
	})
}

// CG-02, CG-03: neither share alone contains content or file names.
func TestSharesHoldNoPlaintext(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	marker := []byte("SECRET-MARKER-XORDOCK")
	name := "SECRET-FILE-NAME"
	if err := v.Mkdir("/" + name + "-dir"); err != nil {
		t.Fatal(err)
	}
	if err := v.PutFile("/"+name+"-dir/"+name+".txt", content(5*BlockSize, string(marker))); err != nil {
		t.Fatal(err)
	}
	scanFor(t, a, marker, []byte(name))
	scanFor(t, b, marker, []byte(name))
}

type recordingMasks struct {
	mu    sync.Mutex
	seen  map[[32]byte]int
	fills int
	first [][]byte
}

func (r *recordingMasks) Fill(p []byte) error {
	if err := (split.CSPRNG{}).Fill(p); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen == nil {
		r.seen = map[[32]byte]int{}
	}
	r.seen[sha256.Sum256(p)]++
	r.fills++
	if len(p) >= 8 {
		r.first = append(r.first, append([]byte(nil), p[:8]...))
	}
	return nil
}

// CG-04, FR-07, FR-08, FR-21: every version of every block has a new mask,
// even when writing identical content.
func TestEveryWriteGetsFreshMask(t *testing.T) {
	a, b := roots(t)
	rm := &recordingMasks{}
	v := mustCreate(t, a, b, Options{Masks: rm})
	data := content(2*BlockSize, "same")
	var blocks [][]string
	for i := 0; i < 5; i++ {
		if err := v.PutFile("/plik", data); err != nil {
			t.Fatal(err)
		}
		e, _ := v.Stat("/plik")
		blocks = append(blocks, e.Blocks)
	}
	for h, n := range rm.seen {
		if n > 1 {
			t.Fatalf("mask %x used %d times", h[:4], n)
		}
	}
	// 1 index at initialization + 5 × (2 blocks + 1 index)
	if rm.fills != 1+5*3 {
		t.Fatalf("expected 16 masks, got %d", rm.fills)
	}
	ids := map[string]bool{}
	for _, bs := range blocks {
		for _, id := range bs {
			if ids[id] {
				t.Fatal("the same object in two versions of the file")
			}
			ids[id] = true
		}
	}
}

// Old versions are deleted (share B first), so the mask history on the card
// does not grow beyond the current and previous state.
func TestOldVersionsCollected(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	for i := 0; i < 10; i++ {
		if err := v.PutFile("/plik", content(BlockSize, fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	count := func(root string) int {
		n := 0
		filepath.WalkDir(filepath.Join(root, objDir), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	// current + previous: 1 block and 1 index each
	if na, nb := count(a), count(b); na != 4 || nb != 4 {
		t.Fatalf("expected 4 objects per side, got A=%d B=%d", na, nb)
	}
}

// CG-06, FR-17: share B of another vault is rejected.
func TestWrongCardRejected(t *testing.T) {
	a1, b1 := roots(t)
	a2, b2 := roots(t)
	mustCreate(t, a1, b1, Options{})
	mustCreate(t, a2, b2, Options{})
	if _, err := Open(a1, b2, Options{}); !errors.Is(err, ErrWrongCard) {
		t.Fatalf("expected ErrWrongCard, got %v", err)
	}
}

// CG-05: the vault cannot be opened without share B.
func TestMissingCardBlocksOpen(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/x", []byte("data"))
	os.Rename(b, b+"-removed")
	if _, err := Open(a, b, Options{}); err == nil {
		t.Fatal("vault opened without the card")
	}
}

// FR-14: a damaged share gives an error, not random data.
func TestCorruptShareIsError(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/x", content(1000, "data"))
	e, _ := v.Stat("/x")
	id, _ := ParseID(e.Blocks[0])
	p := objPath(b, id)
	raw, _ := os.ReadFile(p)
	raw[prefixLen+10] ^= 0xff
	os.WriteFile(p, raw, 0o600)
	if _, err := v.GetFile("/x"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt, got %v", err)
	}
	if r := v.Fsck(); r.OK() {
		t.Fatal("fsck did not detect the damage")
	}
}

func TestCorruptSuperIsError(t *testing.T) {
	a, b := roots(t)
	mustCreate(t, a, b, Options{})
	os.WriteFile(filepath.Join(a, superFile), []byte("garbage\n00\n"), 0o600)
	if _, err := Open(a, b, Options{}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt, got %v", err)
	}
}

var errCrash = errors.New("simulated failure")

// CG-07, SC-05, G-05: a crash at any commit stage ends with
// the old or the new version after reopening, never something in between.
func TestCrashAtEveryStage(t *testing.T) {
	stages := []string{"share-A", "share-B", "barrier", "super-A", "super-B"}
	oldData := content(2*BlockSize+3, "old")
	newData := content(3*BlockSize+7, "new!")
	for _, stage := range stages {
		for nth := 1; nth <= 6; nth++ { // 4 blocks + index = 5 share writes
			t.Run(fmt.Sprintf("%s#%d", stage, nth), func(t *testing.T) {
				a, b := roots(t)
				v := mustCreate(t, a, b, Options{})
				if err := v.PutFile("/plik", oldData); err != nil {
					t.Fatal(err)
				}
				hits := 0
				v.hook = func(s string) error {
					if s == stage {
						hits++
						if hits == nth {
							return errCrash
						}
					}
					return nil
				}
				err := v.PutFile("/plik", newData)
				if hits < nth {
					t.Skip("the stage did not occur that many times")
				}
				if err == nil {
					t.Fatal("write reported success despite the crash")
				}
				if err := v.PutFile("/other", []byte("x")); !errors.Is(err, ErrBroken) {
					t.Fatalf("after the crash expected ErrBroken, got %v", err)
				}
				v2 := mustOpen(t, a, b)
				got, err := v2.GetFile("/plik")
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case bytes.Equal(got, oldData):
				case bytes.Equal(got, newData) && stage == "super-B":
					// superblock A written: commit completed on open
				default:
					t.Fatalf("after a crash at %s the content is neither the old nor the expected version", stage)
				}
				if r := v2.Fsck(); !r.OK() {
					t.Fatal(r.Problems)
				}
			})
		}
	}
}

// A crash halfway through writing superblock B (temporary file) is cleaned up.
func TestTornTempFilesCleaned(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/x", []byte("data"))
	os.WriteFile(filepath.Join(b, superFile+".tmp"), []byte("half"), 0o600)
	mustOpen(t, a, b)
	if _, err := os.Stat(filepath.Join(b, superFile+".tmp")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("temporary file was not deleted")
	}
}

// An interrupted initialization (no superblock B) is completed (FR-06).
func TestInterruptedInitFinished(t *testing.T) {
	a, b := roots(t)
	_, err := Create(a, b, "t", Options{Hook: func(s string) error {
		if s == "super-B" {
			return errCrash
		}
		return nil
	}})
	if err == nil {
		t.Fatal("expected an error")
	}
	v := mustOpen(t, a, b)
	if v.Recovery == "" {
		t.Fatal("no recovery information")
	}
}

// CG-08: concurrent writes of different files do not mix shares.
func TestConcurrentWrites(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := v.PutFile(fmt.Sprintf("/p%d", i), content(BlockSize+i*1000, fmt.Sprintf("<%d>", i))); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	v2 := mustOpen(t, a, b)
	for i := 0; i < 16; i++ {
		got, err := v2.GetFile(fmt.Sprintf("/p%d", i))
		if err != nil || !bytes.Equal(got, content(BlockSize+i*1000, fmt.Sprintf("<%d>", i))) {
			t.Fatalf("file %d: %v", i, err)
		}
	}
}

// CG-09: logs contain no content, file names or masks.
func TestLogsHoldNoSecrets(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	old := Logf
	Logf = func(f string, args ...any) { mu.Lock(); fmt.Fprintf(&buf, f+"\n", args...); mu.Unlock() }
	defer func() { Logf = old }()

	a, b := roots(t)
	rm := &recordingMasks{}
	v := mustCreate(t, a, b, Options{Masks: rm})
	v.PutFile("/SECRET-NAME", content(BlockSize, "SECRET-MARKER"))
	v.hook = func(s string) error {
		if s == "super-B" {
			return errCrash
		}
		return nil
	}
	v.PutFile("/SECRET-NAME", content(BlockSize, "SECRET-MARKER-2"))
	mustOpen(t, a, b).Close()
	// orphaned object, so that the GC log fires
	os.MkdirAll(filepath.Join(a, objDir, "00"), 0o700)
	os.WriteFile(filepath.Join(a, objDir, "00", "00"+strings.Repeat("1", 30)), []byte("x"), 0o600)
	mustOpen(t, a, b)

	logs := buf.String()
	if logs == "" {
		t.Fatal("the test produced no logs")
	}
	for _, s := range []string{"SECRET-NAME", "SECRET-MARKER"} {
		if strings.Contains(logs, s) {
			t.Fatalf("log contains %q", s)
		}
	}
	for _, m := range rm.first {
		if strings.Contains(logs, hex.EncodeToString(m)) {
			t.Fatal("log contains a mask fragment")
		}
	}
}

func TestRenameAndRemove(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.Mkdir("/d")
	v.Mkdir("/d/e")
	v.PutFile("/d/e/f", []byte("content"))
	if err := v.Rename("/d", "/z"); err != nil {
		t.Fatal(err)
	}
	if got, err := v.GetFile("/z/e/f"); err != nil || string(got) != "content" {
		t.Fatalf("after move: %q %v", got, err)
	}
	if _, ok := v.Stat("/d/e/f"); ok {
		t.Fatal("old path still exists")
	}
	if err := v.Rename("/z", "/z/e/w"); err == nil {
		t.Fatal("moving a directory into itself should fail")
	}
	if err := v.RemoveAll("/z"); err != nil {
		t.Fatal(err)
	}
	if len(v.Paths()) != 1 {
		t.Fatalf("entries left: %v", v.Paths())
	}
	if err := v.PutFile("/missing/file", nil); !errors.Is(err, ErrNotExist) {
		t.Fatalf("write without a parent directory: %v", err)
	}
}

// A second process cannot open the same vault (the GC of one would delete
// objects written by the other).
func TestSecondOpenLocked(t *testing.T) {
	a, b := roots(t)
	mustCreate(t, a, b, Options{})
	v := mustOpen(t, a, b)
	if _, err := Open(a, b, Options{}); err == nil {
		t.Fatal("second open should fail")
	}
	v.Close()
	mustOpen(t, a, b).Close()
}

// File numbers are stable: they survive renames, writing new content
// and reopening the vault (needed for macOS file system integration).
func TestInodesStable(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	if err := v.Mkdir("/d"); err != nil {
		t.Fatal(err)
	}
	ino, err := v.Create("/d/f", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Create("/d/f", true); !errors.Is(err, ErrExist) {
		t.Fatalf("exclusive create of an existing file: %v", err)
	}
	if again, err := v.Create("/d/f", false); err != nil || again != ino {
		t.Fatalf("non-exclusive create: %d %v", again, err)
	}
	if _, err := v.RenameReplacing("/d", "/e"); err != nil {
		t.Fatal(err)
	}
	blocks, _ := v.WriteBlocks([]byte("content"))
	if err := v.SetFileIno(ino, blocks, 5, nil); err != nil {
		t.Fatal(err)
	}
	if p, e, ok := v.ByIno(ino); !ok || p != "/e/f" || e.Size != 5 {
		t.Fatalf("ByIno after move: %q %+v %v", p, e, ok)
	}
	v2 := mustOpen(t, a, b)
	defer v2.Close()
	if e, _ := v2.Stat("/e/f"); e.Ino != ino {
		t.Fatalf("number after reopening: %d, was %d", e.Ino, ino)
	}
	if root, _ := v2.Stat("/"); root.Ino != 1 {
		t.Fatalf("root directory has number %d", root.Ino)
	}
}

// Replacing a file on move returns the replaced file's number; Remove
// has unlink/rmdir semantics.
func TestReplaceAndRemoveSemantics(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	x, _ := v.Create("/x", true)
	y, _ := v.Create("/y", true)
	if replaced, err := v.RenameReplacing("/x", "/y"); err != nil || replaced != y {
		t.Fatalf("replaced %d (expected %d): %v", replaced, y, err)
	}
	if e, _ := v.Stat("/y"); e.Ino != x {
		t.Fatal("file has a different number after move")
	}
	v.Mkdir("/d")
	v.Create("/d/z", true)
	if _, err := v.Remove("/d", true); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("rmdir of a non-empty directory: %v", err)
	}
	if _, err := v.Remove("/d", false); !errors.Is(err, ErrIsDir) {
		t.Fatalf("unlink of a directory: %v", err)
	}
	if _, err := v.Remove("/d/z", false); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Remove("/d", true); err != nil {
		t.Fatal(err)
	}
}

// An index from before file numbers were introduced gets them deterministically.
func TestEnsureInosForOldIndex(t *testing.T) {
	ix := &Index{Entries: map[string]*Entry{"/": {Dir: true}, "/b": {}, "/a": {}}}
	ix.ensureInos()
	if ix.Entries["/"].Ino != 1 || ix.Entries["/a"].Ino != 2 || ix.Entries["/b"].Ino != 3 || ix.NextIno != 4 {
		t.Fatalf("numbers: / %d, /a %d, /b %d, next %d", ix.Entries["/"].Ino, ix.Entries["/a"].Ino, ix.Entries["/b"].Ino, ix.NextIno)
	}
}
