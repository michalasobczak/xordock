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
	SameDeviceCheck = false // w testach A i B leżą w jednym katalogu tymczasowym
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

// CG-01: odczyt po zapisie i po ponownym otwarciu daje identyczne bajty.
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
			t.Fatalf("rozmiar %d: treść się różni", n)
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
				t.Errorf("%s zawiera jawną treść %q", p, n)
			}
		}
		return nil
	})
}

// CG-02, CG-03: żaden z udziałów osobno nie zawiera treści ani nazw plików.
func TestSharesHoldNoPlaintext(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	marker := []byte("TAJNY-MARKER-XORDOCK")
	name := "TAJNA-NAZWA-PLIKU"
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

// CG-04, FR-07, FR-08, FR-21: każda wersja każdego bloku ma nową maskę,
// także przy zapisie identycznej treści.
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
			t.Fatalf("maska %x użyta %d razy", h[:4], n)
		}
	}
	// 1 indeks przy inicjalizacji + 5 × (2 bloki + 1 indeks)
	if rm.fills != 1+5*3 {
		t.Fatalf("oczekiwano 16 masek, było %d", rm.fills)
	}
	ids := map[string]bool{}
	for _, bs := range blocks {
		for _, id := range bs {
			if ids[id] {
				t.Fatal("ten sam obiekt w dwóch wersjach pliku")
			}
			ids[id] = true
		}
	}
}

// Stare wersje są usuwane (najpierw udział B), więc na karcie nie rośnie
// historia masek ponad bieżący i poprzedni stan.
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
	// bieżący + poprzedni: po 1 bloku i 1 indeksie
	if na, nb := count(a), count(b); na != 4 || nb != 4 {
		t.Fatalf("oczekiwano 4 obiektów na stronę, jest A=%d B=%d", na, nb)
	}
}

// CG-06, FR-17: udział B innego sejfu jest odrzucany.
func TestWrongCardRejected(t *testing.T) {
	a1, b1 := roots(t)
	a2, b2 := roots(t)
	mustCreate(t, a1, b1, Options{})
	mustCreate(t, a2, b2, Options{})
	if _, err := Open(a1, b2, Options{}); !errors.Is(err, ErrWrongCard) {
		t.Fatalf("oczekiwano ErrWrongCard, jest %v", err)
	}
}

// CG-05: bez udziału B sejfu nie da się otworzyć.
func TestMissingCardBlocksOpen(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/x", []byte("dane"))
	os.Rename(b, b+"-wyjęta")
	if _, err := Open(a, b, Options{}); err == nil {
		t.Fatal("sejf otwarty bez karty")
	}
}

// FR-14: uszkodzony udział daje błąd, nie przypadkowe dane.
func TestCorruptShareIsError(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/x", content(1000, "dane"))
	e, _ := v.Stat("/x")
	id, _ := ParseID(e.Blocks[0])
	p := objPath(b, id)
	raw, _ := os.ReadFile(p)
	raw[prefixLen+10] ^= 0xff
	os.WriteFile(p, raw, 0o600)
	if _, err := v.GetFile("/x"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("oczekiwano ErrCorrupt, jest %v", err)
	}
	if r := v.Fsck(); r.OK() {
		t.Fatal("fsck nie wykrył uszkodzenia")
	}
}

func TestCorruptSuperIsError(t *testing.T) {
	a, b := roots(t)
	mustCreate(t, a, b, Options{})
	os.WriteFile(filepath.Join(a, superFile), []byte("śmieci\n00\n"), 0o600)
	if _, err := Open(a, b, Options{}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("oczekiwano ErrCorrupt, jest %v", err)
	}
}

var errCrash = errors.New("symulowana awaria")

// CG-07, SC-05, G-05: awaria na każdym etapie zatwierdzania kończy się
// starą albo nową wersją po ponownym otwarciu, nigdy czymś pośrednim.
func TestCrashAtEveryStage(t *testing.T) {
	stages := []string{"share-A", "share-B", "barrier", "super-A", "super-B"}
	oldData := content(2*BlockSize+3, "stara")
	newData := content(3*BlockSize+7, "nowa!")
	for _, stage := range stages {
		for nth := 1; nth <= 6; nth++ { // 4 bloki + indeks = 5 wystąpień zapisu udziału
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
					t.Skip("etap nie wystąpił tyle razy")
				}
				if err == nil {
					t.Fatal("zapis zgłosił sukces mimo awarii")
				}
				if err := v.PutFile("/inny", []byte("x")); !errors.Is(err, ErrBroken) {
					t.Fatalf("po awarii oczekiwano ErrBroken, jest %v", err)
				}
				v2 := mustOpen(t, a, b)
				got, err := v2.GetFile("/plik")
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case bytes.Equal(got, oldData):
				case bytes.Equal(got, newData) && stage == "super-B":
					// superblok A zapisany: commit dokończony przy otwarciu
				default:
					t.Fatalf("po awarii na %s treść nie jest ani starą, ani oczekiwaną wersją", stage)
				}
				if r := v2.Fsck(); !r.OK() {
					t.Fatal(r.Problems)
				}
			})
		}
	}
}

// Awaria w połowie zapisu superbloku B (plik tymczasowy) jest sprzątana.
func TestTornTempFilesCleaned(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/x", []byte("dane"))
	os.WriteFile(filepath.Join(b, superFile+".tmp"), []byte("połowa"), 0o600)
	mustOpen(t, a, b)
	if _, err := os.Stat(filepath.Join(b, superFile+".tmp")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("plik tymczasowy nie został usunięty")
	}
}

// Przerwana inicjalizacja (brak superbloku B) jest dokańczana (FR-06).
func TestInterruptedInitFinished(t *testing.T) {
	a, b := roots(t)
	_, err := Create(a, b, "t", Options{Hook: func(s string) error {
		if s == "super-B" {
			return errCrash
		}
		return nil
	}})
	if err == nil {
		t.Fatal("oczekiwano błędu")
	}
	v := mustOpen(t, a, b)
	if v.Recovery == "" {
		t.Fatal("brak informacji o odzyskaniu")
	}
}

// CG-08: równoległe zapisy różnych plików nie mieszają udziałów.
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
			t.Fatalf("plik %d: %v", i, err)
		}
	}
}

// CG-09: logi nie zawierają treści, nazw plików ani masek.
func TestLogsHoldNoSecrets(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	old := Logf
	Logf = func(f string, args ...any) { mu.Lock(); fmt.Fprintf(&buf, f+"\n", args...); mu.Unlock() }
	defer func() { Logf = old }()

	a, b := roots(t)
	rm := &recordingMasks{}
	v := mustCreate(t, a, b, Options{Masks: rm})
	v.PutFile("/TAJNA-NAZWA", content(BlockSize, "TAJNY-MARKER"))
	v.hook = func(s string) error {
		if s == "super-B" {
			return errCrash
		}
		return nil
	}
	v.PutFile("/TAJNA-NAZWA", content(BlockSize, "TAJNY-MARKER-2"))
	mustOpen(t, a, b).Close()
	// osierocony obiekt, żeby zadziałał log GC
	os.MkdirAll(filepath.Join(a, objDir, "00"), 0o700)
	os.WriteFile(filepath.Join(a, objDir, "00", "00"+strings.Repeat("1", 30)), []byte("x"), 0o600)
	mustOpen(t, a, b)

	logs := buf.String()
	if logs == "" {
		t.Fatal("test nie wygenerował logów")
	}
	for _, s := range []string{"TAJNA-NAZWA", "TAJNY-MARKER"} {
		if strings.Contains(logs, s) {
			t.Fatalf("log zawiera %q", s)
		}
	}
	for _, m := range rm.first {
		if strings.Contains(logs, hex.EncodeToString(m)) {
			t.Fatal("log zawiera fragment maski")
		}
	}
}

func TestRenameAndRemove(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.Mkdir("/d")
	v.Mkdir("/d/e")
	v.PutFile("/d/e/f", []byte("treść"))
	if err := v.Rename("/d", "/z"); err != nil {
		t.Fatal(err)
	}
	if got, err := v.GetFile("/z/e/f"); err != nil || string(got) != "treść" {
		t.Fatalf("po przeniesieniu: %q %v", got, err)
	}
	if _, ok := v.Stat("/d/e/f"); ok {
		t.Fatal("stara ścieżka nadal istnieje")
	}
	if err := v.Rename("/z", "/z/e/w"); err == nil {
		t.Fatal("przeniesienie katalogu do wnętrza powinno się nie udać")
	}
	if err := v.RemoveAll("/z"); err != nil {
		t.Fatal(err)
	}
	if len(v.Paths()) != 1 {
		t.Fatalf("zostały wpisy: %v", v.Paths())
	}
	if err := v.PutFile("/brak/plik", nil); !errors.Is(err, ErrNotExist) {
		t.Fatalf("zapis bez katalogu nadrzędnego: %v", err)
	}
}

// Drugi proces nie może otworzyć tego samego sejfu (GC jednego usunąłby
// obiekty zapisywane przez drugi).
func TestSecondOpenLocked(t *testing.T) {
	a, b := roots(t)
	mustCreate(t, a, b, Options{})
	v := mustOpen(t, a, b)
	if _, err := Open(a, b, Options{}); err == nil {
		t.Fatal("drugie otwarcie powinno się nie udać")
	}
	v.Close()
	mustOpen(t, a, b).Close()
}
