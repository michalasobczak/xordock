package main

import (
	"os"
	"path/filepath"
	"testing"

	"xordock/internal/vault"
)

func TestMain(m *testing.M) {
	vault.SameDeviceCheck = false
	os.Exit(m.Run())
}

// S007 U-01: lista sejfów łączy udział A z kartą znalezioną po identyfikatorze,
// także gdy wolumin karty ma inną nazwę niż sejf.
func TestCollectVaults(t *testing.T) {
	d := t.TempDir()
	dir, vols := filepath.Join(d, "app"), filepath.Join(d, "Volumes")
	a := filepath.Join(dir, "sejf")
	b := filepath.Join(vols, "PENDRIVE", "xordock", "sejf")
	v, err := vault.Create(a, b, "sejf", vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	v.PutFile("/f", []byte("abc"))
	got := collectVaults(dir, vols)
	if len(got) != 1 || got[0].Card != b || got[0].Seq != v.Seq() || !got[0].BackupDue || got[0].Error != "" {
		t.Fatalf("%+v", got)
	}
	v.Close()
	os.RemoveAll(filepath.Join(vols, "PENDRIVE"))
	got = collectVaults(dir, vols)
	if got[0].Card != "" || got[0].Error != "" {
		t.Fatalf("bez karty: %+v", got[0])
	}
}

func TestWholeDisk(t *testing.T) {
	for in, want := range map[string]string{"/dev/disk3s1": "disk3", "/dev/disk12s3s1": "disk12", "map auto_home": "", "/dev/disk5": "disk5"} {
		if got := wholeDisk(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}

// Nazwa sejfu nie może wyprowadzić ścieżek A/B poza ich katalogi.
func TestValidName(t *testing.T) {
	for _, bad := range []string{"", "..", ".ukryty", "a/b", "../x", "a:b", " a", "a ", "a\nb", string(make([]rune, 65))} {
		if validName(bad) == nil {
			t.Errorf("przyjęto %q", bad)
		}
	}
	for _, good := range []string{"test", "Moje dokumenty", "zdjęcia-2026", "a.b_c"} {
		if err := validName(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
}

// S008 K-05: kopia A w iCloud jest widoczna przy sejfie, a bez udziału A
// na Macu trafia na listę kopii do odzyskania.
func TestACopyInState(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "app", "sejf"), filepath.Join(d, "Volumes", "K", "xordock", "sejf")
	spare := filepath.Join(d, "Volumes", "Z", "xordock", "sejf")
	icloud := filepath.Join(d, "icloud", "xordock")
	v, err := vault.Create(a, b, "sejf", vault.Options{})
	if err != nil {
		t.Fatal(err)
	}
	v.PutFile("/f", []byte("abc"))
	v.CreateSnapshot()
	if _, _, err := v.BackupPair(spare, filepath.Join(icloud, "sejf"), ""); err != nil {
		t.Fatal(err)
	}
	v.Close()
	jc, ok := readCopy(filepath.Join(icloud, "sejf"))
	if !ok || jc.ID != v.ID().String() || jc.Points != 1 || jc.LastPoint == "" {
		t.Fatalf("%+v", jc)
	}
	if _, ok := readCopy(a); ok {
		t.Fatal("żywy udział A uznany za kopię")
	}
	vs := collectVaults(filepath.Join(d, "app"), filepath.Join(d, "Volumes"))
	if got := orphanCopies(icloud, vs); len(got) != 0 {
		t.Fatalf("kopia z udziałem A na Macu jako osierocona: %+v", got)
	}
	os.RemoveAll(filepath.Join(d, "app"))
	if got := orphanCopies(icloud, nil); len(got) != 1 || got[0].Name != "sejf" {
		t.Fatalf("po utracie SSD: %+v", got)
	}
}
