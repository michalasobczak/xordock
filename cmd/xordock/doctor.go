package main

// xordock doctor: requirements check (S011). Text output or JSON for
// the app. Test mount only with --probe-mount.

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

// System Settings → General → Login Items & Extensions.
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

// versionAtLeast compares "a.b.c" versions numerically.
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

// Substitution points for tests (S011 G3: no macFUSE without uninstalling it).
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
		add(check{ID: "macos", Title: "macOS 26 or later", Level: "ok", Detail: "macOS " + osv})
	} else {
		add(check{ID: "macos", Title: "macOS 26 or later", Level: "error",
			Detail: fmt.Sprintf("You have macOS %s. The macFUSE FSKit module requires macOS 26.", osv)})
	}

	ver := plistValue(macfusePlist, "CFBundleShortVersionString")
	r.MacFUSEVersion = ver
	switch {
	case ver == "" || !macfuseAvailable():
		add(check{ID: "macfuse", Title: "macFUSE 5.5 or later", Level: "error",
			Detail: "macFUSE is not installed. Without it the vault cannot be opened in Finder.", FixLabel: "Download macFUSE", FixURL: macfuseURL})
	case !versionAtLeast(ver, "5.5"):
		add(check{ID: "macfuse", Title: "macFUSE 5.5 or later", Level: "error",
			Detail: "Installed version " + ver + " is too old.", FixLabel: "Download macFUSE", FixURL: macfuseURL})
	default:
		add(check{ID: "macfuse", Title: "macFUSE 5.5 or later", Level: "ok", Detail: "macFUSE " + ver})
	}

	ext := check{ID: "extension", Title: "macFUSE file system extension", Level: "unknown",
		Detail: "Not checked. Checking requires a test mount."}
	if probe && ver != "" && macfuseAvailable() {
		if err := probeMount(15 * time.Second); err != nil {
			ext.Level = "error"
			ext.Detail = "Mounting failed. Enable macFUSE in System Settings → General → Login Items & Extensions → File System Extensions."
			ext.FixLabel, ext.FixURL = "Open System Settings", extensionsURL
		} else {
			ext.Level, ext.Detail = "ok", "Test mount succeeded."
		}
	}
	if ver == "" || !macfuseAvailable() {
		ext.Detail = "Will be checked after macFUSE is installed."
	}
	add(ext)

	if fileVaultOn() {
		add(check{ID: "filevault", Title: "FileVault", Level: "ok", Detail: "On."})
	} else {
		add(check{ID: "filevault", Title: "FileVault", Level: "warning",
			Detail:   "Off. Apps may leave plain copies of vault files on the SSD.",
			FixLabel: "Open FileVault settings", FixURL: "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?FileVault"})
	}
	if iCloudAvailable() {
		add(check{ID: "icloud", Title: "iCloud Drive", Level: "ok", Detail: "The share A copy goes to iCloud Drive."})
	} else {
		add(check{ID: "icloud", Title: "iCloud Drive", Level: "warning",
			Detail:   "Unavailable. Backup will not copy share A, so losing the Mac = losing the data.",
			FixLabel: "Open iCloud settings", FixURL: "x-apple.systempreferences:com.apple.systempreferences.AppleIDSettings:icloud"})
	}
	return r
}

func cmdDoctor(args []string) error {
	fl := flag.NewFlagSet("doctor", flag.ExitOnError)
	probe := fl.Bool("probe-mount", false, "check the macFUSE extension with a test mount (empty volume, a few seconds)")
	asJSON := fl.Bool("json", false, "JSON output")
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
		return fmt.Errorf("required components are missing")
	}
	return nil
}
