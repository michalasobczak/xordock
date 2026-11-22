package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func spareRoot(t *testing.T, a string) string {
	return filepath.Join(filepath.Dir(a), "SPARE")
}

func mustGet(t *testing.T, r FileReader, p string) []byte {
	t.Helper()
	got, err := r.GetFile(p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return got
}

// CG-06, CG-07: migawka przypina stan G1; zapis G2 nie zmienia odczytu G1,
// a GC nie usuwa obiektów migawki. Migawka nie kopiuje danych.
func TestSnapshotKeepsOldVersion(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	g1 := content(2*BlockSize+3, "wersja-pierwsza")
	if err := v.PutFile("/doc", g1); err != nil {
		t.Fatal(err)
	}
	before := countObjects(t, a)
	g1e, _ := v.Stat("/doc")
	s, err := v.CreateSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	// migawka = tylko nowy indeks, bez kopii bloków
	if got := countObjects(t, a); got > before+1 {
		t.Fatalf("migawka skopiowała dane: %d → %d obiektów", before, got)
	}
	for i := 0; i < 3; i++ {
		if err := v.PutFile("/doc", content(BlockSize, "wersja-druga")); err != nil {
			t.Fatal(err)
		}
	}
	v.Close()
	v2 := mustOpen(t, a, b) // także gcAll przy otwarciu
	defer v2.Close()
	w, err := v2.SnapshotView(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustGet(t, w, "/doc"), g1) {
		t.Fatal("migawka G1 zwraca inne bajty")
	}
	if !bytes.Equal(mustGet(t, v2, "/doc"), content(BlockSize, "wersja-druga")) {
		t.Fatal("stan bieżący zły")
	}
	if ch, _ := v2.Changes(s.ID); len(ch) != 1 || ch[0] != "/doc" {
		t.Fatalf("zmiany od migawki: %v", ch)
	}
	if r := v2.Fsck(); !r.OK() || r.Snapshots != 1 {
		t.Fatalf("fsck: %+v", r)
	}
	// usunięcie migawki zwalnia jej obiekty
	if err := v2.DeleteSnapshot(s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.SnapshotView(s.ID); !errors.Is(err, ErrNoSnapshot) {
		t.Fatal("migawka nadal istnieje")
	}
	// Poprzedni stan (punkt powrotu) wciąż wskazuje migawkę; po kolejnym
	// commicie jej bloki muszą zniknąć z obu stron.
	v2.Mkdir("/x")
	for _, h := range g1e.Blocks {
		oid, _ := ParseID(h)
		for _, root := range []string{a, b} {
			if _, err := os.Stat(objPath(root, oid)); err == nil {
				t.Fatalf("GC nie zwolnił bloku migawki w %s", root)
			}
		}
	}
}

func countObjects(t *testing.T, root string) int {
	n := 0
	filepath.WalkDir(filepath.Join(root, objDir), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// S-04: gdy indeksu migawki nie da się odczytać, GC niczego nie usuwa.
func TestGCStopsWhenSnapshotUnreadable(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/f", content(100, "x"))
	s, _ := v.CreateSnapshot()
	v.PutFile("/f", content(100, "y"))
	v.Close()
	sid, _ := ParseID(s.Index)
	os.Remove(objPath(b, sid)) // uszkodzony indeks migawki
	before := countObjects(t, a)
	v2 := mustOpen(t, a, b)
	defer v2.Close()
	v2.PutFile("/f", content(100, "z"))
	if got := countObjects(t, a); got < before {
		t.Fatalf("GC usuwał mimo niepewności: %d → %d", before, got)
	}
	if r := v2.Fsck(); r.OK() {
		t.Fatal("fsck nie zgłosił uszkodzonej migawki")
	}
}

// CG-08, SC-04: utrata karty roboczej → odtworzenie dokładnych bajtów G1
// z migawki A i karty zapasowej; późniejsze zmiany nie wracają.
func TestRecoverAfterLosingWorkingCard(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	g1 := content(3*BlockSize+11, "dane-g1")
	v.Mkdir("/kat")
	v.PutFile("/kat/plik", g1)
	s, _ := v.CreateSnapshot()
	p, err := v.Backup(spare, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Snapshot != s.ID || p.State != stateReady {
		t.Fatalf("punkt: %+v", p)
	}
	if ref, _, ok := v.LastBackup(); !ok || !ref.Ready || ref.Point != p.ID {
		t.Fatalf("status backupu: %+v %v", ref, ok)
	}
	v.PutFile("/kat/plik", content(10, "po-backupie")) // G2, nie ma go na zapasowej
	v.PutFile("/nowy", content(10, "po-backupie"))
	v.Close()
	if err := os.RemoveAll(b); err != nil { // zgubiona karta robocza
		t.Fatal(err)
	}
	if _, err := Open(a, b, Options{}); err == nil {
		t.Fatal("sejf otworzył się bez karty roboczej")
	}
	if _, err := Open(a, spare, Options{}); !errors.Is(err, ErrSpareCard) {
		t.Fatalf("karta zapasowa użyta jako robocza: %v", err) // CG-09
	}
	r, err := OpenRecovery(a, spare)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	pts, bad := r.Points()
	if len(pts) != 1 || len(bad) != 0 {
		t.Fatalf("punkty: %v %v", pts, bad)
	}
	if _, n, err := r.Verify(""); err != nil || n != 1 {
		t.Fatalf("weryfikacja: %d %v", n, err)
	}
	w, _, err := r.View("")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustGet(t, w, "/kat/plik"), g1) {
		t.Fatal("odzyskane bajty różnią się od G1")
	}
	if _, ok := w.Stat("/nowy"); ok {
		t.Fatal("punkt odzyskiwania zawiera zmianę sprzed niego niezapisaną")
	}
	// Nowy sejf z nowymi maskami (R-04).
	d := t.TempDir()
	nv, err := Create(filepath.Join(d, "A2"), filepath.Join(d, "B2"), "odzyskany", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := CopyInto(w, nv); err != nil || n != 1 {
		t.Fatalf("kopiowanie: %d %v", n, err)
	}
	if !bytes.Equal(mustGet(t, nv, "/kat/plik"), g1) {
		t.Fatal("nowy sejf ma inne bajty")
	}
	// Eksport jawny (rollback).
	out := filepath.Join(d, "eksport")
	if n, err := ExportTo(w, out); err != nil || n != 1 {
		t.Fatalf("eksport: %d %v", n, err)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "kat", "plik")); !bytes.Equal(got, g1) {
		t.Fatal("eksport ma inne bajty")
	}
}

// CG-09: karta zapasowa B_G1 nie składa obiektów G2 ani niczego spoza punktu.
func TestSpareNotUsedForNewerObjects(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/f", content(100, "g1"))
	v.CreateSnapshot()
	if _, err := v.Backup(spare, ""); err != nil {
		t.Fatal(err)
	}
	v.PutFile("/f", content(100, "g2"))
	e, _ := v.Stat("/f")
	v.Close()
	r, err := OpenRecovery(a, spare)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	w, _, _ := r.View("")
	oid, _ := ParseID(e.Blocks[0])
	if _, err := w.read(oid); err == nil {
		t.Fatal("punkt G1 złożył obiekt z G2")
	}
	if !bytes.Equal(mustGet(t, w, "/f"), content(100, "g1")) {
		t.Fatal("G1 złe")
	}
}

// CG-10: przerwanie backupu na każdym etapie zostawia poprzedni READY
// i nie tworzy punktu READY z przerwanej próby.
func TestInterruptedBackupKeepsPreviousPoint(t *testing.T) {
	for _, stage := range []string{"backup-copy", "backup-verify", "backup-ready"} {
		t.Run(stage, func(t *testing.T) {
			a, b := roots(t)
			spare := spareRoot(t, a)
			fail := ""
			v := mustCreate(t, a, b, Options{Hook: func(s string) error {
				if s == fail {
					return errors.New("awaria: " + s)
				}
				return nil
			}})
			defer v.Close()
			v.PutFile("/f", content(2*BlockSize, "g1"))
			v.CreateSnapshot()
			p1, err := v.Backup(spare, "")
			if err != nil {
				t.Fatal(err)
			}
			v.PutFile("/f", content(2*BlockSize, "g2"))
			v.CreateSnapshot()
			fail = stage
			if _, err := v.Backup(spare, ""); err == nil {
				t.Fatal("backup nie zgłosił awarii")
			}
			fail = ""
			pts, _ := ReadPoints(spare, v.ID())
			if len(pts) != 1 || pts[0].ID != p1.ID {
				t.Fatalf("punkty po awarii: %+v", pts)
			}
			ix, err := loadIndexFrom(a, spare, v.ID(), p1.Index)
			if err != nil {
				t.Fatal(err)
			}
			w := &View{ix: ix, read: func(id ID) ([]byte, error) { return readPair(a, spare, v.ID(), id) }}
			if !bytes.Equal(mustGet(t, w, "/f"), content(2*BlockSize, "g1")) {
				t.Fatal("poprzedni punkt uszkodzony")
			}
			// Po awarii kolejny backup się udaje.
			if _, err := v.Backup(spare, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// CG-11: brak miejsca → błąd, nic nie skopiowano, poprzedni punkt cały.
func TestBackupNoSpace(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(100, "g1"))
	v.CreateSnapshot()
	p1, err := v.Backup(spare, "")
	if err != nil {
		t.Fatal(err)
	}
	v.PutFile("/g", content(3*BlockSize, "g2"))
	v.CreateSnapshot()
	before := countObjects(t, spare)
	old := FreeSpace
	FreeSpace = func(string) (uint64, error) { return 1 << 20, nil }
	defer func() { FreeSpace = old }()
	if _, err := v.Backup(spare, ""); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("oczekiwano ErrNoSpace, jest %v", err)
	}
	if countObjects(t, spare) != before {
		t.Fatal("backup kopiował mimo braku miejsca")
	}
	if pts, _ := ReadPoints(spare, v.ID()); len(pts) != 1 || pts[0].ID != p1.ID {
		t.Fatal("poprzedni punkt zmieniony")
	}
}

// B-04, S-04: migawka z punktem jest przypięta; prune zostawia najnowszy
// punkt i zdejmuje przypięcia starszych.
func TestPruneKeepsNewestAndUnpins(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(BlockSize, "g1"))
	s1, _ := v.CreateSnapshot()
	v.Backup(spare, "")
	if err := v.DeleteSnapshot(s1.ID); !errors.Is(err, ErrPinned) {
		t.Fatalf("przypięta migawka usunięta: %v", err)
	}
	v.PutFile("/f", content(BlockSize, "g2"))
	v.CreateSnapshot()
	p2, err := v.Backup(spare, "")
	if err != nil {
		t.Fatal(err)
	}
	np, no, err := v.PruneBackup(spare)
	if err != nil || np != 1 || no == 0 {
		t.Fatalf("prune: %d %d %v", np, no, err)
	}
	if pts, _ := ReadPoints(spare, v.ID()); len(pts) != 1 || pts[0].ID != p2.ID {
		t.Fatal("prune nie zostawił najnowszego punktu")
	}
	if err := v.DeleteSnapshot(s1.ID); err != nil {
		t.Fatalf("migawka po prune nadal przypięta: %v", err)
	}
	v.Close()
	r, err := OpenRecovery(a, spare)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, _, err := r.Verify(""); err != nil {
		t.Fatalf("najnowszy punkt po prune: %v", err)
	}
}

// CG-12: karta zapasowa i rekordy punktów nie zawierają treści ani nazw.
func TestSpareHoldsNoPlaintext(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	marker := []byte("TAJNY-MARKER-BACKUP")
	name := "TAJNA-NAZWA-BACKUP"
	v.PutFile("/"+name, content(2*BlockSize, string(marker)))
	v.CreateSnapshot()
	if _, err := v.Backup(spare, ""); err != nil {
		t.Fatal(err)
	}
	scanFor(t, spare, marker, []byte(name))
	scanFor(t, a, marker, []byte(name))
}

// Backup odmawia wskazania karty roboczej jako zapasowej.
func TestBackupRejectsWorkingCard(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(10, "x"))
	v.CreateSnapshot()
	if _, err := v.Backup(b, ""); err == nil {
		t.Fatal("karta robocza przyjęta jako zapasowa")
	}
}

// S005 / R4-01: prune nie usuwa starszych punktów, gdy najnowszy jest
// niekompletny (B-04: nigdy nie tracimy jedynej dobrej kopii).
func TestPruneRefusesIncompleteNewest(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(BlockSize, "g1"))
	v.CreateSnapshot()
	p1, _ := v.Backup(spare, "")
	v.PutFile("/f", content(BlockSize, "g2"))
	v.CreateSnapshot()
	v.Backup(spare, "")
	e, _ := v.Stat("/f")
	oid, _ := ParseID(e.Blocks[0])
	os.WriteFile(objPath(spare, oid), []byte("uszkodzony"), 0o600) // blok najnowszego punktu
	if _, _, err := v.PruneBackup(spare); err == nil {
		t.Fatal("prune zadziałał mimo niekompletnego najnowszego punktu")
	}
	pts, _ := ReadPoints(spare, v.ID())
	if len(pts) != 2 || pts[1].ID != p1.ID {
		t.Fatalf("prune usunął punkty: %+v", pts)
	}
}

// S005 / R4-02: indeks ze ścieżką wychodzącą poza drzewo jest odrzucany,
// a eksport nie zapisuje poza katalogiem docelowym.
func TestIndexPathsValidated(t *testing.T) {
	for _, bad := range []string{"/../ucieczka", "/a/../../b", "wzgledna", "/a//b", "/a/"} {
		ix := newIndex()
		ix.Entries[bad] = &Entry{Ino: 5}
		if err := ix.validate(); err == nil {
			t.Fatalf("indeks ze ścieżką %q przyjęty", bad)
		}
	}
	ix := newIndex()
	ix.Entries["/k"] = &Entry{Ino: 2, Dir: true}
	ix.Entries["/k/p"] = &Entry{Ino: 3}
	if err := ix.validate(); err != nil {
		t.Fatal(err)
	}
	ix.Entries["/brak/rodzica"] = &Entry{Ino: 4}
	if err := ix.validate(); err == nil {
		t.Fatal("wpis bez katalogu nadrzędnego przyjęty")
	}
}
