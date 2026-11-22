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

// S008 G1, G2: losing the SSD. After deleting the A directory the point's data comes back
// byte for byte from the A copy + spare card, and also from the A copy + working card.
func TestRecoverAfterLosingSSD(t *testing.T) {
	a, b := roots(t)
	spare, acp := spareRoot(t, a), aCopyRoot(a)
	v := mustCreate(t, a, b, Options{})
	g1 := content(3*BlockSize+11, "before-ssd-loss")
	if err := v.Mkdir("/docs"); err != nil {
		t.Fatal(err)
	}
	if err := v.PutFile("/docs/file", g1); err != nil {
		t.Fatal(err)
	}
	v.PutFile("/small", []byte("abc"))
	v.CreateSnapshot()
	pb, pa, err := v.BackupPair(spare, acp, "")
	if err != nil {
		t.Fatal(err)
	}
	if pb.ID != pa.ID || pb.Snapshot != pa.Snapshot || pb.Media == pa.Media {
		t.Fatalf("pair points: %+v / %+v", pb, pa)
	}
	if ref, _, ok := v.LastBackup(); !ok || !ref.Ready {
		t.Fatal("no READY pin")
	}
	v.PutFile("/docs/file", []byte("change after backup"))
	v.Close()
	if err := os.RemoveAll(a); err != nil { // SSD lost
		t.Fatal(err)
	}
	for _, bSrc := range []string{spare, b} {
		r, err := OpenRecovery(acp, bSrc)
		if err != nil {
			t.Fatalf("%s: %v", bSrc, err)
		}
		if _, n, err := r.Verify(""); err != nil || n != 2 {
			t.Fatalf("%s: verify %d %v", bSrc, n, err)
		}
		w, p, err := r.View("")
		if err != nil || p.ID != pa.ID {
			t.Fatal(err)
		}
		if !bytes.Equal(mustGet(t, w, "/docs/file"), g1) || string(mustGet(t, w, "/small")) != "abc" {
			t.Fatalf("%s: different bytes", bSrc)
		}
		r.Close()
	}
	if _, err := os.Stat(filepath.Join(acp, "lock")); err == nil {
		t.Fatal("recovery changed the A copy (lock)")
	}
	// Restore to a new vault with new masks.
	r, _ := OpenRecovery(acp, spare)
	defer r.Close()
	w, _, _ := r.View("")
	d := t.TempDir()
	nv := mustCreate(t, filepath.Join(d, "A2"), filepath.Join(d, "B2"), Options{})
	defer nv.Close()
	if _, err := CopyInto(w, nv); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustGet(t, nv, "/docs/file"), g1) {
		t.Fatal("restore from the A copy")
	}
}

// S008 G3: the A copy holds only side A shares, the header and point records;
// no B share and no plain content or names.
func TestACopyHoldsNoBNoPlaintext(t *testing.T) {
	a, b := roots(t)
	spare, acp := spareRoot(t, a), aCopyRoot(a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	marker, name := []byte("SECRET-MARKER-ICLOUD"), "SECRET-NAME-ICLOUD"
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
			t.Errorf("%s is not a share A: %v", d.Name(), err)
		}
		if _, err := decodeShare(raw, SideB, v.ID(), id); err == nil {
			t.Errorf("%s is a share B", d.Name())
		}
		n++
		return nil
	})
	if n == 0 {
		t.Fatal("empty A copy")
	}
	if h, err := readHeader(acp, SideA); err != nil || h.Role != roleBackup {
		t.Fatalf("A copy header: %+v %v", h, err)
	}
	// The A copy cannot be opened as a live vault or as a B card.
	if _, err := Open(acp, b, Options{}); err == nil {
		t.Fatal("A copy opened as a live share")
	}
}

// S008 G4: the A copy target cannot be inside a card or share directory.
func TestACopyRejectsCardLocations(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", []byte("x"))
	v.CreateSnapshot()
	for _, bad := range []string{filepath.Join(b, "copy"), filepath.Join(spare, "copy"), b, filepath.Join(a, "copy")} {
		if _, _, err := v.BackupPair(spare, bad, ""); !errors.Is(err, ErrACopyOnCard) {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

// S008 G5: interrupting the A copy leaves the previous A point recoverable,
// and the new B point is READY.
func TestInterruptedACopy(t *testing.T) {
	for _, stage := range []string{"acopy-copy", "acopy-verify", "acopy-ready"} {
		t.Run(stage, func(t *testing.T) {
			a, b := roots(t)
			spare, acp := spareRoot(t, a), aCopyRoot(a)
			fail := ""
			v := mustCreate(t, a, b, Options{Hook: func(s string) error {
				if s == fail {
					return errors.New("failure: " + s)
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
				t.Fatal("no error")
			}
			fail = ""
			if pts, _ := ReadPoints(spare, v.ID()); len(pts) != 2 || pts[0].ID != pb.ID {
				t.Fatalf("B point after the A copy failure: %+v", pts)
			}
			pa, _ := ReadPoints(acp, v.ID())
			if len(pa) != 1 || pa[0].ID != p1.ID {
				t.Fatalf("A points after the failure: %+v", pa)
			}
			r, err := OpenRecovery(acp, spare)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			w, p, err := r.View("")
			if err != nil || p.ID != p1.ID || !bytes.Equal(mustGet(t, w, "/f"), g1) {
				t.Fatalf("previous A point: %v %v", p.ID, err)
			}
		})
	}
}

// S008 G6: prune keeps only the newest point on both targets, and it can still
// be recovered without the SSD.
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
		t.Fatal("newest point after prune", err)
	}
}
