package fusefs

import (
	"bytes"
	"math/rand/v2"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"xordock/internal/vault"
)

func TestMain(m *testing.M) {
	vault.SameDeviceCheck = false
	os.Exit(m.Run())
}

func setup(t *testing.T) (*FS, string, string) {
	d := t.TempDir()
	a, b := filepath.Join(d, "A"), filepath.Join(d, "B")
	v, err := vault.Create(a, b, "t", vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return New(v), a, b
}

func ok(t *testing.T, e syscall.Errno) {
	t.Helper()
	if e != 0 {
		t.Fatal(e)
	}
}

// CG-01, CG-04 przez adapter: losowe zapisy i obcięcia dają te same bajty co
// model, także po zamknięciu i ponownym otwarciu sejfu.
func TestRandomWritesMatchModel(t *testing.T) {
	f, a, b := setup(t)
	fh, e := f.Create("/plik")
	ok(t, e)
	var model []byte
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 300; i++ {
		switch r.IntN(10) {
		case 0:
			sz := int64(r.IntN(3 * bs))
			ok(t, f.Truncate("/plik", fh, sz))
			if sz < int64(len(model)) {
				model = model[:sz]
			} else {
				model = append(model, make([]byte, sz-int64(len(model)))...)
			}
		case 1:
			ok(t, f.Flush(fh))
		default:
			off := int64(r.IntN(3 * bs))
			data := make([]byte, r.IntN(200000)+1)
			for j := range data {
				data[j] = byte(r.Uint32())
			}
			n, e := f.Write(fh, off, data)
			ok(t, e)
			if n != len(data) {
				t.Fatal("krótki zapis")
			}
			if end := off + int64(len(data)); end > int64(len(model)) {
				model = append(model, make([]byte, end-int64(len(model)))...)
			}
			copy(model[off:], data)
		}
		got, e := f.Read(fh, 0, len(model)+10)
		ok(t, e)
		if !bytes.Equal(got, model) {
			t.Fatalf("krok %d: odczyt różni się od modelu (%d vs %d B)", i, len(got), len(model))
		}
	}
	ok(t, f.Release(fh))
	f.v.Close()
	v2, err := vault.Open(a, b, vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	got, err := v2.GetFile("/plik")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, model) {
		t.Fatal("po ponownym otwarciu treść inna")
	}
	if r := v2.Fsck(); !r.OK() {
		t.Fatal(r.Problems)
	}
}

// Duży plik: pełne bloki trafiają do obiektów przed commitem (spill).
func TestLargeFileSpills(t *testing.T) {
	f, _, _ := setup(t)
	fh, e := f.Create("/duzy")
	ok(t, e)
	chunk := bytes.Repeat([]byte("0123456789abcdef"), 1<<14) // 256 KiB
	total := 0
	for total < (maxDirty+8)*bs {
		_, e := f.Write(fh, int64(total), chunk)
		ok(t, e)
		total += len(chunk)
	}
	if n := f.handles[fh]; len(n.dirty) > maxDirty {
		t.Fatalf("w RAM %d bloków, limit %d", len(n.dirty), maxDirty)
	}
	ok(t, f.Release(fh))
	got, err := f.v.GetFile("/duzy")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != total || !bytes.Equal(got[:len(chunk)], chunk) || !bytes.Equal(got[total-len(chunk):], chunk) {
		t.Fatal("treść dużego pliku zła")
	}
}

// F-03: operacje katalogowe, rename z flagami, jawne błędy.
func TestDirectoryOps(t *testing.T) {
	f, _, _ := setup(t)
	ok(t, f.Mkdir("/k"))
	if e := f.Mkdir("/k"); e != syscall.EEXIST {
		t.Fatalf("mkdir istniejącego: %v", e)
	}
	fh, e := f.Create("/k/a")
	ok(t, e)
	f.Write(fh, 0, []byte("treść"))
	ok(t, f.Release(fh))
	if e := f.Rmdir("/k"); e != syscall.ENOTEMPTY {
		t.Fatalf("rmdir niepustego: %v", e)
	}
	ok(t, f.Rename("/k/a", "/k/b", 0))
	fh2, _ := f.Create("/k/c")
	f.Release(fh2)
	if e := f.Rename("/k/b", "/k/c", RenameExcl); e != syscall.EEXIST {
		t.Fatalf("rename EXCL na istniejący: %v", e)
	}
	if e := f.Rename("/k/b", "/k/c", RenameSwap); e != syscall.ENOTSUP {
		t.Fatalf("rename SWAP: %v", e)
	}
	ents, e := f.Readdir("/k")
	ok(t, e)
	if len(ents) != 2 || ents[0].Name != "b" || ents[0].Attr.Size != int64(len("treść")) {
		t.Fatalf("readdir: %+v", ents)
	}
	ok(t, f.Unlink("/k/b"))
	ok(t, f.Unlink("/k/c"))
	ok(t, f.Rmdir("/k"))
	if _, e := f.Getattr("/k"); e != syscall.ENOENT {
		t.Fatalf("getattr usuniętego: %v", e)
	}
	mt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	fh3, _ := f.Create("/t")
	f.Release(fh3)
	ok(t, f.Utimens("/t", mt))
	at, _ := f.Getattr("/t")
	if !at.Mtime.Equal(mt) {
		t.Fatalf("mtime %v", at.Mtime)
	}
}

// O_TRUNC przy otwarciu i obcięcie nieotwartego pliku.
func TestTruncateClosedAndOpenTrunc(t *testing.T) {
	f, _, _ := setup(t)
	f.v.PutFile("/x", bytes.Repeat([]byte("a"), bs+10))
	ok(t, f.Truncate("/x", 0, 5))
	if got, _ := f.v.GetFile("/x"); string(got) != "aaaaa" {
		t.Fatalf("po truncate: %q", got)
	}
	fh, e := f.Open("/x", true)
	ok(t, e)
	f.Write(fh, 0, []byte("b"))
	ok(t, f.Release(fh))
	if got, _ := f.v.GetFile("/x"); string(got) != "b" {
		t.Fatalf("po O_TRUNC: %q", got)
	}
}

// SC-03, F-04: wyjęcie karty → każda operacja EIO, bufory wyczyszczone,
// niezatwierdzone zmiany nie trafiają do sejfu.
func TestCardRemovedIsDeterministicError(t *testing.T) {
	f, _, _ := setup(t)
	fh, _ := f.Create("/p")
	f.Write(fh, 0, []byte("niezatwierdzone"))
	n := f.handles[fh]
	buf := n.dirty[0]
	f.Unavailable(syscall.ENXIO)
	if _, e := f.Read(fh, 0, 10); e != syscall.EIO {
		t.Fatalf("odczyt po wyjęciu karty: %v", e)
	}
	if _, e := f.Write(fh, 0, []byte("x")); e != syscall.EIO {
		t.Fatalf("zapis po wyjęciu karty: %v", e)
	}
	if _, e := f.Getattr("/p"); e != syscall.EIO {
		t.Fatalf("getattr po wyjęciu karty: %v", e)
	}
	if _, e := f.Getattr("/"); e != 0 {
		t.Fatal("katalog główny musi odpowiadać, żeby dało się odmontować")
	}
	if !bytes.Equal(buf[:15], make([]byte, 15)) {
		t.Fatal("bufor z jawną treścią nie został wyczyszczony")
	}
	f.Release(fh)
	if f.SyncAll() == nil {
		t.Fatal("SyncAll po wyjęciu karty zgłosił sukces")
	}
}

// Usunięcie otwartego pliku: jego zmiany nie wskrzeszają go przy zamknięciu.
func TestUnlinkWhileOpen(t *testing.T) {
	f, _, _ := setup(t)
	fh, _ := f.Create("/u")
	f.Write(fh, 0, []byte("abc"))
	ok(t, f.Unlink("/u"))
	ok(t, f.Release(fh))
	if _, ok := f.v.Stat("/u"); ok {
		t.Fatal("usunięty plik wrócił")
	}
}
