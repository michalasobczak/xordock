package vault

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func aCopyRoot(a string) string { return filepath.Join(filepath.Dir(a), "ICLOUD", "xordock", "t") }

// S008 G1, G2: utrata SSD. Po usunięciu katalogu A dane punktu wracają
// bajt w bajt z kopii A + karty zapasowej, a także z kopii A + karty roboczej.
func TestRecoverAfterLosingSSD(t *testing.T) {
	a, b := roots(t)
	spare, acp := spareRoot(t, a), aCopyRoot(a)
	v := mustCreate(t, a, b, Options{})
	g1 := content(3*BlockSize+11, "przed-utrata-ssd")
	if err := v.Mkdir("/dok"); err != nil {
		t.Fatal(err)
	}
	if err := v.PutFile("/dok/plik", g1); err != nil {
		t.Fatal(err)
	}
	v.PutFile("/maly", []byte("abc"))
	v.CreateSnapshot()
	pb, pa, err := v.BackupPair(spare, acp, "")
	if err != nil {
		t.Fatal(err)
	}
	if pb.ID != pa.ID || pb.Snapshot != pa.Snapshot || pb.Media == pa.Media {
		t.Fatalf("punkty pary: %+v / %+v", pb, pa)
	}
	if ref, _, ok := v.LastBackup(); !ok || !ref.Ready {
		t.Fatal("brak przypięcia READY")
	}
	v.PutFile("/dok/plik", []byte("zmiana po backupie"))
	v.Close()
	if err := os.RemoveAll(a); err != nil { // SSD utracony
		t.Fatal(err)
	}
	for _, bSrc := range []string{spare, b} {
		r, err := OpenRecovery(acp, bSrc)
		if err != nil {
			t.Fatalf("%s: %v", bSrc, err)
		}
		if _, n, err := r.Verify(""); err != nil || n != 2 {
			t.Fatalf("%s: weryfikacja %d %v", bSrc, n, err)
		}
		w, p, err := r.View("")
		if err != nil || p.ID != pa.ID {
			t.Fatal(err)
		}
		if !bytes.Equal(mustGet(t, w, "/dok/plik"), g1) || string(mustGet(t, w, "/maly")) != "abc" {
			t.Fatalf("%s: inne bajty", bSrc)
		}
		r.Close()
	}
	if _, err := os.Stat(filepath.Join(acp, "lock")); err == nil {
		t.Fatal("odzyskiwanie zmieniło kopię A (lock)")
	}
	// Odtworzenie do nowego sejfu z nowymi maskami.
	r, _ := OpenRecovery(acp, spare)
	defer r.Close()
	w, _, _ := r.View("")
	d := t.TempDir()
	nv := mustCreate(t, filepath.Join(d, "A2"), filepath.Join(d, "B2"), Options{})
	defer nv.Close()
	if _, err := CopyInto(w, nv); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustGet(t, nv, "/dok/plik"), g1) {
		t.Fatal("restore z kopii A")
	}
}

// S008 G3: w kopii A są tylko udziały strony A, nagłówek i rekordy punktów;
// żadnego udziału B ani jawnej treści czy nazw.
func TestACopyHoldsNoBNoPlaintext(t *testing.T) {
	a, b := roots(t)
	spare, acp := spareRoot(t, a), aCopyRoot(a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	marker, name := []byte("TAJNY-MARKER-ICLOUD"), "TAJNA-NAZWA-ICLOUD"
	v.PutFile("/"+name, content(2*BlockSize, string(marker)))
	v.CreateSnapshot()
	if _, _, err := v.BackupPair(spare, acp, ""); err != nil {
		t.Fatal(err)
	}
	scanFor(t, acp, marker, []byte(name))
	n := 0
	filepath.WalkDir(filepath.Join(acp, objDir), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		id, _ := ParseID(d.Name())
		raw, _ := os.ReadFile(p)
		if _, err := decodeShare(raw, SideA, v.ID(), id); err != nil {
			t.Errorf("%s nie jest udziałem A: %v", d.Name(), err)
		}
		if _, err := decodeShare(raw, SideB, v.ID(), id); err == nil {
			t.Errorf("%s jest udziałem B", d.Name())
		}
		n++
		return nil
	})
	if n == 0 {
		t.Fatal("pusta kopia A")
	}
	if h, err := readHeader(acp, SideA); err != nil || h.Role != roleBackup {
		t.Fatalf("nagłówek kopii A: %+v %v", h, err)
	}
	// Kopia A nie daje się otworzyć jako żywy sejf ani jako karta B.
	if _, err := Open(acp, b, Options{}); err == nil {
		t.Fatal("kopia A otwarta jako żywy udział")
	}
}

// S008 G4: cel kopii A nie może leżeć w katalogu karty ani udziału.
func TestACopyRejectsCardLocations(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", []byte("x"))
	v.CreateSnapshot()
	for _, bad := range []string{filepath.Join(b, "kopia"), filepath.Join(spare, "kopia"), b, filepath.Join(a, "kopia")} {
		if _, _, err := v.BackupPair(spare, bad, ""); !errors.Is(err, ErrACopyOnCard) {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

// S008 G5: przerwanie kopii A zostawia poprzedni punkt A odzyskiwalny,
// a nowy punkt B jest READY.
func TestInterruptedACopy(t *testing.T) {
	for _, stage := range []string{"acopy-copy", "acopy-verify", "acopy-ready"} {
		t.Run(stage, func(t *testing.T) {
			a, b := roots(t)
			spare, acp := spareRoot(t, a), aCopyRoot(a)
			fail := ""
			v := mustCreate(t, a, b, Options{Hook: func(s string) error {
				if s == fail {
					return errors.New("awaria: " + s)
				}
				return nil
			}})
			defer v.Close()
			g1 := content(2*BlockSize, "g1")
			v.PutFile("/f", g1)
			v.CreateSnapshot()
			_, p1, err := v.BackupPair(spare, acp, "")
			if err != nil {
				t.Fatal(err)
			}
			v.PutFile("/f", content(2*BlockSize, "g2"))
			v.CreateSnapshot()
			fail = stage
			pb, _, err := v.BackupPair(spare, acp, "")
			if err == nil {
				t.Fatal("brak błędu")
			}
			fail = ""
			if pts, _ := ReadPoints(spare, v.ID()); len(pts) != 2 || pts[0].ID != pb.ID {
				t.Fatalf("punkt B po awarii kopii A: %+v", pts)
			}
			pa, _ := ReadPoints(acp, v.ID())
			if len(pa) != 1 || pa[0].ID != p1.ID {
				t.Fatalf("punkty A po awarii: %+v", pa)
			}
			r, err := OpenRecovery(acp, spare)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			w, p, err := r.View("")
			if err != nil || p.ID != p1.ID || !bytes.Equal(mustGet(t, w, "/f"), g1) {
				t.Fatalf("poprzedni punkt A: %v %v", p.ID, err)
			}
		})
	}
}

// S008 G6: prune zostawia na obu celach tylko najnowszy punkt, a ten dalej
// daje się odzyskać bez SSD.
func TestPruneBothCopies(t *testing.T) {
	a, b := roots(t)
	spare, acp := spareRoot(t, a), aCopyRoot(a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(2*BlockSize, "g1"))
	v.CreateSnapshot()
	v.BackupPair(spare, acp, "")
	v.PutFile("/f", content(2*BlockSize, "g2"))
	v.CreateSnapshot()
	_, p2, err := v.BackupPair(spare, acp, "")
	if err != nil {
		t.Fatal(err)
	}
	before := countObjects(t, acp)
	if np, _, err := v.PruneBackup(spare); err != nil || np != 1 {
		t.Fatal(np, err)
	}
	np, no, err := v.PruneACopy(acp)
	if err != nil || np != 1 || no == 0 || countObjects(t, acp) >= before {
		t.Fatal(np, no, err)
	}
	for _, d := range []string{spare, acp} {
		if pts, _ := ReadPoints(d, v.ID()); len(pts) != 1 || pts[0].ID != p2.ID {
			t.Fatalf("%s: %+v", d, pts)
		}
	}
	r, err := OpenRecovery(acp, spare)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	w, _, err := r.View("")
	if err != nil || !bytes.Equal(mustGet(t, w, "/f"), content(2*BlockSize, "g2")) {
		t.Fatal("najnowszy punkt po prune", err)
	}
}
