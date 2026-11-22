package main

// xordock doctor: sprawdzenie wymagań (S011). Wynik tekstowy albo JSON dla
// aplikacji. Próbne montowanie tylko z --probe-mount.

import (
	"flag"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"xordock/internal/fusemount"
)

const macfuseURL = "https://macfuse.github.io/"

// Ustawienia systemowe → Ogólne → Rozszerzenia i elementy logowania.
const extensionsURL = "x-apple.systempreferences:com.apple.LoginItems-Settings.extension"

type check struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Level    string `json:"level"` // ok | warning | error | unknown
	Detail   string `json:"detail"`
	FixLabel string `json:"fixLabel,omitempty"`
	FixURL   string `json:"fixURL,omitempty"`
}

type doctorResult struct {
	MacFUSEVersion string  `json:"macfuseVersion,omitempty"`
	Checks         []check `json:"checks"`
}

// versionAtLeast porównuje wersje „a.b.c” liczbowo.
func versionAtLeast(v, min string) bool {
	vs, ms := strings.Split(v, "."), strings.Split(min, ".")
	for i := range ms {
		var a, b int
		if i < len(vs) {
			a, _ = strconv.Atoi(vs[i])
		}
		b, _ = strconv.Atoi(ms[i])
		if a != b {
			return a > b
		}
	}
	return true
}

// Punkty podmiany w testach (S011 G3: brak macFUSE bez odinstalowania).
var (
	macfusePlist     = "/Library/Filesystems/macfuse.fs/Contents/Info.plist"
	macfuseAvailable = fusemount.Available
	probeMount       = fusemount.Probe
	osVersion        = func() string {
		out, err := exec.Command("/usr/bin/sw_vers", "-productVersion").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
)

func plistValue(path, key string) string {
	out, err := exec.Command("/usr/bin/plutil", "-extract", key, "raw", "-o", "-", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runDoctor(probe bool) doctorResult {
	var r doctorResult
	add := func(c check) { r.Checks = append(r.Checks, c) }

	osv := osVersion()
	if versionAtLeast(osv, "26.0") {
		add(check{ID: "macos", Title: "macOS 26 lub nowszy", Level: "ok", Detail: "macOS " + osv})
	} else {
		add(check{ID: "macos", Title: "macOS 26 lub nowszy", Level: "error",
			Detail: fmt.Sprintf("Masz macOS %s. Moduł FSKit macFUSE wymaga macOS 26.", osv)})
	}

	ver := plistValue(macfusePlist, "CFBundleShortVersionString")
	r.MacFUSEVersion = ver
	switch {
	case ver == "" || !macfuseAvailable():
		add(check{ID: "macfuse", Title: "macFUSE 5.5 lub nowszy", Level: "error",
			Detail: "macFUSE nie jest zainstalowany. Bez niego sejfu nie da się otworzyć w Finderze.", FixLabel: "Pobierz macFUSE", FixURL: macfuseURL})
	case !versionAtLeast(ver, "5.5"):
		add(check{ID: "macfuse", Title: "macFUSE 5.5 lub nowszy", Level: "error",
			Detail: "Zainstalowana wersja " + ver + " jest za stara.", FixLabel: "Pobierz macFUSE", FixURL: macfuseURL})
	default:
		add(check{ID: "macfuse", Title: "macFUSE 5.5 lub nowszy", Level: "ok", Detail: "macFUSE " + ver})
	}

	ext := check{ID: "extension", Title: "Rozszerzenie systemu plików macFUSE", Level: "unknown",
		Detail: "Nie sprawdzono. Sprawdzenie wymaga próbnego montowania."}
	if probe && ver != "" && macfuseAvailable() {
		if err := probeMount(15 * time.Second); err != nil {
			ext.Level = "error"
			ext.Detail = "Montowanie się nie udało. Włącz macFUSE w Ustawieniach systemowych → Ogólne → Rozszerzenia i elementy logowania → Rozszerzenia systemu plików."
			ext.FixLabel, ext.FixURL = "Otwórz Ustawienia systemowe", extensionsURL
		} else {
			ext.Level, ext.Detail = "ok", "Próbne montowanie udane."
		}
	}
	if ver == "" || !macfuseAvailable() {
		ext.Detail = "Sprawdzę po zainstalowaniu macFUSE."
	}
	add(ext)

	if fileVaultOn() {
		add(check{ID: "filevault", Title: "FileVault", Level: "ok", Detail: "Włączony."})
	} else {
		add(check{ID: "filevault", Title: "FileVault", Level: "warning",
			Detail:   "Wyłączony. Programy mogą zostawiać jawne kopie plików z sejfu na SSD.",
			FixLabel: "Otwórz ustawienia FileVault", FixURL: "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?FileVault"})
	}
	if iCloudAvailable() {
		add(check{ID: "icloud", Title: "iCloud Drive", Level: "ok", Detail: "Kopia udziału A trafia do iCloud Drive."})
	} else {
		add(check{ID: "icloud", Title: "iCloud Drive", Level: "warning",
			Detail:   "Niedostępny. Backup nie zrobi kopii udziału A, więc utrata Maca = utrata danych.",
			FixLabel: "Otwórz ustawienia iCloud", FixURL: "x-apple.systempreferences:com.apple.systempreferences.AppleIDSettings:icloud"})
	}
	return r
}

func cmdDoctor(args []string) error {
	fl := flag.NewFlagSet("doctor", flag.ExitOnError)
	probe := fl.Bool("probe-mount", false, "sprawdź rozszerzenie macFUSE próbnym montowaniem (pusty wolumen, kilka sekund)")
	asJSON := fl.Bool("json", false, "wynik JSON")
	fl.Parse(args)
	r := runDoctor(*probe)
	if *asJSON {
		return printJSON(r)
	}
	mark := map[string]string{"ok": "✓", "warning": "⚠︎", "error": "✗", "unknown": "?"}
	bad := false
	for _, c := range r.Checks {
		fmt.Printf("%s %s: %s\n", mark[c.Level], c.Title, c.Detail)
		if c.FixURL != "" && c.Level != "ok" {
			fmt.Printf("    → %s: %s\n", c.FixLabel, c.FixURL)
		}
		bad = bad || c.Level == "error"
	}
	if bad {
		return fmt.Errorf("brakuje wymaganych składników")
	}
	return nil
}
