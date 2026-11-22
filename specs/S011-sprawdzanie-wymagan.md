# S011 — Requirements check at launch

- **Status:** delivered
- **Date:** 2026-11-22
- **Related:** S004 (A-12, ADR-003), S007, S008, S010

## 1. TL;DR

At launch the app checks whether all the tools are present:

- macOS 26+;
- macFUSE 5.5+;
- the macFUSE file system extension enabled;
- a working `xordock` engine;
- FileVault and iCloud Drive as recommended.

If something is missing, it shows a list with a fix button: download macFUSE or open System Settings. When everything is fine, it shows nothing.

## 2. Abstract

xordock needs software that the app does not bundle (S004 A-12: macFUSE license). On first launch the owner hit the error "File system extension not enabled", which surfaced only when mounting, while the CLI then printed a misleading success (fixed in S004).

In addition, the CLI was hard-linked against `libfuse3`. Without macFUSE it would not start at all, not even for commands that do not need macFUSE (status, backup, recovery).

The owner asked (2026-11-22) for a check at launch that all the tools are present. Implementation decisions (defaults, to be confirmed):

- **Required (red):** macOS 26+ (the minimum of the macFUSE FSKit module: `LSMinimumSystemVersion` 26.0), macFUSE ≥ 5.5, extension enabled, CLI working.
- **Recommended (orange):** FileVault, iCloud Drive (A copy, S008).
- **Requirements screen:** shown at launch only when something required does not work. It is also available from Settings and from the padlock menu. It comes before the tutorial question.
- **FSKit extension:** its state lives in a protected system container that the app cannot read. We check it with a **probe mount** of an empty read-only volume, which we unmount immediately. We remember the result for the given macFUSE version, and every successful vault mount also confirms it, so the probe is not repeated on every launch.

## 3. Objective

- **Goals:**
  - W-01 `xordock doctor [--probe-mount] [--json]`: a list of checks `{id, title, level: ok|warning|error|unknown, detail, fixLabel, fixURL}`.
  - W-02 The CLI links `libfuse3` weakly. Without macFUSE it starts, `doctor` reports the absence, and `mount` refuses with a clear message instead of a loader crash.
  - W-03 Probe mount: a file system without a vault ("unavailable" state: root directory empty, every other operation EIO), mounted in a temporary directory for at most 15 s.
  - W-04 At launch the app runs `doctor --json`. It adds the probe mount when the required items are fine and the extension state is not confirmed for the current macFUSE version. The "Requirements" sheet has fix buttons and "Check again".
- **Out of scope:** installing macFUSE through the app, checking the cards (the dashboard does that).
- **Success criteria:** the gates from §7.

## 4. Artefact Inventory

| Artefact | Type | State |
| --- | --- | --- |
| `cmd/xordock/doctor.go` | code | to create |
| `internal/fusemount/probe.go`, `bridge*` | code: probe mount, weak linking, `Available()` | to create / to change |
| `internal/fusefs/fs.go` | code: `NewProbe` | to change |
| `macos/Sources/XordockCore/Requirements.swift`, `XordockUI`/`XordockApp` | code | to create / to change |

## 5. Functional Domain Changes

- The probe volume has no access to any vault or card. It is named `xordock-probe`, and its mount point is a temporary directory that is deleted afterwards.
- Missing macFUSE does not block commands that do not mount (recovery after a failure still works).
- Probe result: "ok", "error" (extension disabled or mount failed) or "unknown" (no probe).

## 6. Critical Path

1. `NewProbe` and `Probe` with a test.
2. Weak linking with `Available()`.
3. `doctor`.
4. App.
5. Probe on real macFUSE.

## 7. Control Gates

| # | Gate | Condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | Probe FS | root directory responds, every other operation EIO, none panics without a vault | `go test` | commit |
| G2 | Probe on hardware | `doctor --probe-mount` gives "ok" with the extension enabled; the volume disappears, the temporary directory is deleted | E2E | commit |
| G3 | No macFUSE | CLI with `DYLD_LIBRARY_PATH` without libfuse3 and a substituted path: `doctor` works, `mount` refuses with a message | manual test | commit |
| G4 | App | result decoding, the rule "show the sheet at launch only on error", "probe when unconfirmed" | `swift test` | commit |
| G5 | **Negative** | the probe does not touch vault directories or cards | review + G1 test | commit |

## 8. Expected Outcome

A missing tool is visible right at launch, with a fix button, instead of an error on the first mount.

## 9. Additional Information

- Rejected: reading the FSKit state from system containers (protected, "Operation not permitted") and `pluginkit` (the macFUSE module is not visible there).
- Downloading macFUSE: the official site `https://macfuse.github.io/`. The app only opens it.

## Implementation (2026-11-22)

- **Weak linking:**
  - The `-weak-lfuse3` flag is rejected by the cgo flag allowlist.
  - Instead, `ops_darwin.c` loads `/usr/local/lib/libfuse3.4.dylib` via `dlopen` on the first mount. The path is fixed, with no `DYLD_*` search.
  - 6 symbols are used; `fuse_new` is an inline wrapper, so we call `_fuse_new_31` directly with a version struct as in the header.
  - `otool -L bin/xordock` no longer shows libfuse.
  - `Mount` and `Probe` return `ErrNoMacFUSE` when the library is missing.
- **Probe volume:**
  - `fusefs.NewProbe()` (no vault, "unavailable" state), `FS.IsProbe()` (no commit log on unmount);
  - `fusemount.Probe` mounts in `os.MkdirTemp`, waits up to 15 s for the volume, unmounts and deletes the directory. On macOS 27 with macFUSE 5.5.0 it takes about 1 s.
- **`xordock doctor`:**
  - checks macOS (≥ 26), macFUSE (plist + `dlopen`, ≥ 5.5), the extension (probe only with `--probe-mount`), FileVault, iCloud Drive;
  - text mode exits with a code ≠ 0 when required items are missing;
  - fixed fix URLs: the macFUSE site and System Settings panes.
- **App:**
  - at launch `model.startup()`: `doctor`, and when the extension is unconfirmed for the current macFUSE version, `doctor --probe-mount`;
  - the result is remembered in `UserDefaults fskitConfirmedForMacFUSE`; every successful vault mount confirms it;
  - the "Requirements" sheet is shown at launch only when something is missing and comes before the tutorial question;
  - access: padlock menu "Requirements…" and Settings "Check requirements" (with a probe).
- **Tests:**
  - Go: `TestProbeFS` (G1, G5), `TestDoctor` (G3: no macFUSE, old version, disabled extension, old macOS, version comparison);
  - Swift: `requirementsRules` (G4), 17 in total.
- **E2E (G2):**
  - `doctor --probe-mount`: "Probe mount succeeded", no volume and no temporary directory after the probe;
  - temporary vault on `xor_primary`: mount, write and read through Finder with the library loaded via `dlopen`, unmount; vault removed;
  - at launch the app saved the confirmation for 5.5.0.
- **Renders:** `docs/ui/requirements-light.png`.
