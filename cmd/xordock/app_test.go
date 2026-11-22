package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"xordock/internal/vault"
)

func TestMain(m *testing.M) {
	vault.SameDeviceCheck = false
	os.Exit(m.Run())
}

// S007 U-01: the vault list joins share A with the card found by ID,
// even when the card's volume has a different name than the vault.
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
		t.Fatalf("no card: %+v", got[0])
	}
}

func TestWholeDisk(t *testing.T) {
	for in, want := range map[string]string{"/dev/disk3s1": "disk3", "/dev/disk12s3s1": "disk12", "map auto_home": "", "/dev/disk5": "disk5"} {
		if got := wholeDisk(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}

// A vault name must not lead the A/B paths outside their directories.
func TestValidName(t *testing.T) {
	for _, bad := range []string{"", "..", ".hidden", "a/b", "../x", "a:b", " a", "a ", "a\nb", string(make([]rune, 65))} {
		if validName(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, good := range []string{"test", "My documents", "café-2026", "a.b_c"} {
		if err := validName(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
}

// S008 K-05: the A copy in iCloud is shown with its vault, and without share A
// on the Mac it goes to the list of copies to recover.
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
		t.Fatal("live share A taken for a copy")
	}
	vs := collectVaults(filepath.Join(d, "app"), filepath.Join(d, "Volumes"))
	if got := orphanCopies(icloud, vs); len(got) != 0 {
		t.Fatalf("copy with share A on the Mac listed as orphaned: %+v", got)
	}
	os.RemoveAll(filepath.Join(d, "app"))
	if got := orphanCopies(icloud, nil); len(got) != 1 || got[0].Name != "sejf" {
		t.Fatalf("after losing the SSD: %+v", got)
	}
}

// S011 G3: no macFUSE, old version, extension disabled, old macOS.
func TestDoctor(t *testing.T) {
	saved := []any{macfusePlist, macfuseAvailable, probeMount, osVersion}
	defer func() {
		macfusePlist = saved[0].(string)
		macfuseAvailable = saved[1].(func() bool)
		probeMount = saved[2].(func(time.Duration) error)
		osVersion = saved[3].(func() string)
	}()
	byID := func(r doctorResult) map[string]check {
		m := map[string]check{}
		for _, c := range r.Checks {
			m[c.ID] = c
		}
		return m
	}
	d := t.TempDir()
	plist := filepath.Join(d, "Info.plist")
	write := func(v string) {
		os.WriteFile(plist, []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleShortVersionString</key><string>`+v+`</string></dict></plist>`), 0o600)
	}
	osVersion = func() string { return "27.0" }
	probed := false
	probeMount = func(time.Duration) error { probed = true; return nil }

	macfusePlist = filepath.Join(d, "missing.plist")
	macfuseAvailable = func() bool { return false }
	c := byID(runDoctor(true))
	if c["macfuse"].Level != "error" || c["macfuse"].FixURL != macfuseURL || c["extension"].Level != "unknown" || probed {
		t.Fatalf("no macFUSE: %+v", c)
	}

	write("5.4.1")
	macfusePlist = plist
	macfuseAvailable = func() bool { return true }
	if c := byID(runDoctor(false)); c["macfuse"].Level != "error" || c["extension"].Level != "unknown" {
		t.Fatalf("old version: %+v", c)
	}

	write("5.5.0")
	if c := byID(runDoctor(true)); c["macfuse"].Level != "ok" || c["extension"].Level != "ok" || !probed {
		t.Fatalf("all ok: %+v", c)
	}
	probeMount = func(time.Duration) error { return errors.New("no") }
	if c := byID(runDoctor(true)); c["extension"].Level != "error" || c["extension"].FixURL != extensionsURL {
		t.Fatalf("extension disabled: %+v", c)
	}
	osVersion = func() string { return "15.4" }
	if c := byID(runDoctor(false)); c["macos"].Level != "error" {
		t.Fatalf("old macOS: %+v", c)
	}
	for _, tc := range []struct {
		v, min string
		ok     bool
	}{{"5.5.0", "5.5", true}, {"5.10", "5.5", true}, {"5.4.9", "5.5", false}, {"27.0", "26.0", true}, {"", "26.0", false}} {
		if versionAtLeast(tc.v, tc.min) != tc.ok {
			t.Errorf("versionAtLeast(%q,%q)", tc.v, tc.min)
		}
	}
}
