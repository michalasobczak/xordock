# S007 — macOS app: menu bar and window

- **Status:** delivered (G1–G5, G7, G8); G6 and G9 await the owner's test on USB sticks
- **Date:** 2026-11-22
- **Related:** S004 (CP-08, ADR-002, CG-14, §7), S006

## 1. TL;DR

A SwiftUI app with a menu bar icon and a window replaces typing CLI commands by hand. It can:

- create a vault, picking the medium from a list;
- mount and unmount;
- show the state of the cards and how current the backup is;
- take a snapshot with a backup in one click;
- recover data from the spare card.

There is still one engine: the app runs the bundled `xordock`.

## 2. Abstract

After S004, xordock is a CLI tool. The owner tested it on two USB sticks and hit the typical traps:

- a typo in the volume name;
- `sudo`, which left A files owned by root;
- a confusing `snapshot` without `create`;
- no information on where the shares live.

Conclusion: "it is hard to click through". S004 anticipated a UI as CP-08 (ADR-002: SwiftUI for managing the volume and the media).

Owner's decisions (2026-11-22):

- **Form:** menu bar + window.
- **Engine:** the app calls the CLI; it does not load Go as a library. The mount process lives separately, so a crash or closing of the UI does not break the mount.
- **Scope of the first version:** mount and state, vault creation, snapshots with backup, recovery.
- **Order:** UI before S006.

## 3. Objective

- **Goals:**
  - U-01 list of vaults from `~/Library/Application Support/xordock/*` with their state: working card connected or not, mounted, spare cards, last READY backup and the number of changes since it;
  - U-02 vault creation: name and volume chosen from the list of connected external media, without typing paths;
  - U-03 mount and unmount, "Show in Finder";
  - U-04 "Back up now" = unmount (with consent) → snapshot → backup to the selected or detected spare card;
  - U-05 a reminder in the menu bar when there is no READY point or the last one is older than 7 days (S004 B-01);
  - U-06 recovery: list of points, verification, export to a chosen directory, restore to a new card;
  - U-07 readable errors (text from the CLI) and a log of the mount process.
- **Out of scope:**
  - system notifications;
  - automatic background backup;
  - launch at login;
  - Developer ID signing and notarization (the app is built locally and signed ad hoc);
  - physical medium checks (S006).
- **Success criteria:** the gates from §7 pass, and the owner goes through the scenario init → mount → write → unmount → backup → verify on USB sticks without a terminal.

## 4. Artefact Inventory

| Artefact | Type | State |
| --- | --- | --- |
| `cmd/xordock/app.go` | code: `vaults --json`, `volumes --json`, `points --json` commands for the app | to create |
| `internal/vault/peek.go` | code: reading vault state without a lock (for a mounted vault) | to create |
| `internal/fusemount/mount.go` | code: export `IsMounted` | to change |
| `macos/Package.swift` | Swift Package: `XordockCore` (logic) + `XordockApp` (SwiftUI) | to create |
| `macos/Sources/XordockCore/*` | code: JSON model, running the CLI, reminder rules | to create |
| `macos/Sources/XordockApp/*` | code: SwiftUI views | to create |
| `macos/Tests/XordockCoreTests/*` | tests (Swift Testing) | to create |
| `scripts/build-app.sh` | script: assembles `build/xordock.app` with the CLI in `Contents/Resources` | to create |

## 5. Functional Domain Changes

- **CLI ↔ UI contract:**
  - JSON on stdout only for query commands (`vaults`, `volumes`, `points`). Actions (init, mount, unmount, snapshot create, backup, recover …) return text; an error is an exit code ≠ 0 and a message on stderr, which the UI shows unchanged.
  - Every JSON field is optional on the Swift side, and unknown fields are ignored.
- **No shell:** the UI runs the CLI through `Process` with an argument array, never through `sh -c`, so vault names and paths are not interpreted.
- **Mounting:**
  - The UI starts `xordock mount --name N` as a child process with output to `~/Library/Logs/xordock/N.log`, not to a pipe. The process lives on after the UI closes, and a broken pipe will not kill it with SIGPIPE.
  - The UI determines the "mounted" state through `vaults --json`, which compares the device of the mount point with its parent directory, not by tracking the process.
- **Refreshing:** on NSWorkspace notifications about volumes being mounted and unmounted, after every action and every 30 s.
- **State of a mounted vault:**
  - Mount holds a flock on A, so `vaults` reads the state through `vault.Peek`: superblock and index without a lock, without GC, without recovery.
  - Objects are immutable, but the GC of the mount process may delete an old index during the read. `Peek` then returns an error, and the UI shows "details unavailable" instead of a false state.
- **Backup:**
  - Snapshot and backup require an unmounted vault (S004 A-11). The button asks for consent to unmount.
  - The spare card is chosen from the list of volumes. When the vault already has a spare card (role `backup` with this identifier), it is selected by default. The UI writes to no medium without the user's choice (S004 §7).
- **Recovery:**
  - Works without the working card.
  - Export requires choosing a parent directory and a new name, because the CLI refuses to write into an existing directory. Warning: the export is plaintext.
- **Texts:** masks described as **practical** protection (CG-14), a warning about FileVault being off, the warning "losing a card without a backup = losing data".
- **Data:** vault format unchanged. The UI neither reads nor writes shares or plaintext content; only the CLI does.

## 6. Critical Path

1. JSON commands in the CLI with Go tests.
2. `XordockCore`: decoding, running processes, the reminder rule, with tests.
3. SwiftUI views: menu bar, window with the list of vaults, wizard, backup, recovery.
4. `scripts/build-app.sh`, manual test on USB sticks.

Done when gates G1–G9 pass.

## 7. Control Gates

| # | Gate | Pass condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | Go tests | `go test -race ./...` green, including the new `vaults`/`volumes`/`Peek` tests | test | commit |
| G2 | Swift tests | `swift test` green: JSON decoding, 7-day reminder, command arguments | test | commit |
| G3 | Build | `scripts/build-app.sh` creates an ad-hoc-signed `build/xordock.app` with the CLI inside | command | commit |
| G4 | **Negative:** no shell | no `/bin/sh`, `bash`, `-c` in the Swift code | grep in a test | commit |
| G5 | **Negative:** UI does not touch shares | Swift code does not open files in vault directories (only `Process` + log) | review + grep | commit |
| G6 | Mount survives closing the UI | after the app is closed, the volume keeps working | manual E2E | release |
| G7 | State of a mounted vault | `vaults --json` with a mounted vault returns `mounted:true` and the last backup | Go test + E2E | commit |
| G8 | CG-14 | UI texts speak of practical protection | review | commit |
| G9 | Log | mount log without file names (CG-12) | E2E | release |

## 8. Expected Outcome

- Day-to-day work without a terminal.
- Side effect: a second language (Swift) and a second process are added. The engine is unchanged.
- Rollback: removing `macos/` and the JSON commands changes neither the format nor the CLI.

## 9. Additional Information

- **Rejected:**
  - Go as a c-archive in the UI process: a UI crash breaks the mount, and two runtimes run in one process.
  - An `.xcodeproj` project: a Swift Package + script are enough and do not require an Apple account.
- **Dependency:** macFUSE 5.5 (the CLI links `/usr/local/lib/libfuse3`). The app does not bundle macFUSE (S004 A-12).
- **Debt:**
  - no Developer ID signature: Gatekeeper may warn on first launch from another Mac;
  - no UI tests (XCUITest requires an Xcode project).

## Implementation (2026-11-22)

- **Build:**
  - Swift Package (`macos/Package.swift`, Swift 6.3, tools 5.10, macOS 14+) instead of an Xcode project.
  - `swift test` requires `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer`, because the Command Line Tools lack the Testing module. The `scripts/build-app.sh` script sets this itself.
- **CLI:**
  - `vaults` (state of all vaults + FileVault + description of masks), `volumes` (volumes in `/Volumes` with FS, free space, a `systemDisk` flag and the vaults on the medium), `points --spare`. Times in RFC 3339 with one-second precision.
  - `systemDisk` is a preliminary safeguard in the UI (same whole disk as `/`). Full physical medium checks belong to S006.
- **Engine:** `vault.Peek` reads the state without a lock. When the superblocks point to different commits, it returns `ErrBusy`. The newest READY point is computed by a shared `lastReady` function, which `LastBackup` also uses.
- **Vault name (R7-01):** validated in the CLI (`--name`, `--new-name`) and in the UI. It must not be empty, be longer than 64 characters, start with a dot, contain `/`, `:` or control characters, or start or end with a space. Previously `--name ../x` led A, B and the mount point outside their directories.
- **CLI location (R7-03):** inside the `.app` bundle only `Contents/Resources/xordock`. `XORDOCK_CLI` and `bin/xordock` work only outside the bundle (`swift run`, tests).
- **Mounting:** the `xordock mount` process starts with output to `~/Library/Logs/xordock/<name>.log` (directory 0700, file 0600). The app waits up to 15 s for the volume to appear, then opens it in Finder.
- **Menu bar icon:** `lock.shield` when everything is fine, `lock.open` when something is mounted, `exclamationmark.shield` on a vault error, FileVault off or a backup being required.
- **Gates:**
  - G1: `go test -race ./...` (new: `TestPeekWhileOpen`, `TestCollectVaults`, `TestWholeDisk`, `TestValidName`).
  - G2, G4, G5: 9 Swift tests in `macos/Tests/XordockCoreTests/CoreTests.swift`. G4 and G5 are checked by a test that searches the sources.
  - G3: the script and `codesign --verify --strict`.
  - G7: `TestPeekWhileOpen` (read while the lock is held).
  - G8: texts in the wizard, in the vault details and in the `masks` field.
