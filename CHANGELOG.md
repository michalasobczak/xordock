# CHANGELOG

A chronological record of what was actually delivered. Entries are ordered by date, newest first.

## 2026-11-22

### S013: clear state after the working card is removed

- Pulling the card out of an open vault now produces one sentence ("The working card was removed while vault "X" was open. The vault was locked and unmounted; unsaved changes were lost…") with the mount log under Details, instead of the raw log; the stale "opened in Finder" banner is cleared.
- Without the working card the spare card tile shows "unknown" instead of a false "no backup", and the iCloud tile shows the share A copy date without a warning.
- Swift tests: 4 new or updated, 22 in total.

### README: step-by-step tutorial with real screenshots

- New README section "Tutorial: your first vault, step by step": the six intro screens, creating a vault, the card contents, the vault in Finder, the card next to the open vault, backup, the green dashboard, the share A copy in iCloud Drive and what happens when the card is pulled out while the vault is open, from a real run on two USB sticks.
- Screenshots in `docs/tutorial/` (owner's screenshots; the Finder sidebar with the home folder name and the computer name cropped out before publishing).

### License: AGPL-3.0

- `LICENSE` with the official text of the GNU Affero General Public License v3 (owner's choice); README section "License": AGPL-3.0 or later, copyright Michal Sobczak, macFUSE not bundled and loaded at runtime under its own license.
- Public repository: https://github.com/michalasobczak/xordock

### README rewritten as the project's front page

- New README: what xordock is, why the one-time-pad construction differs from computational encryption, Shannon's perfect-secrecy proof for the 2-of-2 XOR split, and what the proof does not cover (length, integrity, open vault, copies).
- Two protection levels stated explicitly: masks from the kernel API (`SecRandomCopyBytes`, today) give **practical** protection with no resistance to P = NP; masks read directly from a hardware generator (not through the kernel DRBG), with the other conditions met, would give information-theoretic protection. Conditions table with today's status.
- What Macs offer: Apple Silicon keeps its hardware RNG behind the kernel DRBG and has no `FEAT_RNG` (checked on M1 Pro); Intel Macs have `RDSEED`; otherwise an external USB TRNG/QRNG. No claim of P = NP resistance for the current product (S004 CG-14, SC-07).
- Loss scenarios (one and two media: cards, Mac, iCloud account) with confidentiality, availability and what to do; usage for the app and the CLI.

### S012: English version of the project

- All user-facing texts are now English: the macOS app (dashboard, tutorial, requirements, sheets, menu), the CLI (usage, output, errors, logs, `doctor`, mask description) and the build script. No bilingual output.
- Code comments (Go, C, Swift), test texts, README, CHANGELOG, MANIFEST, `docs/`, all specs and the spec template translated. `CLAUDE.md`: artifacts in English, conversation with the owner in Polish.
- `docs/` file names in English: `docs/usage.md`, `docs/security/2026-11-22-round-S00x.md`, `docs/ui/*-light.png`/`*-dark.png`; all links updated. Spec file names unchanged.
- App dates in `en_US`, English plurals. Restore default name `<name>-recovered`; probe volume `xordock-probe`; default vault name `sejf` kept for compatibility.
- After `recover … restore` the CLI now tells you to delete every copy of the old share A (SSD, iCloud Drive incl. Recently Deleted, Time Machine).
- Guard tests: Go `TestSourcesAreEnglish` (string literals and comments, plus the C bridge), Swift `sourcesAreEnglishOnly`, `healthChangesPlural`. Swift tests: 19.

### S011: requirements check at startup

- At startup the app checks:
  - macOS 26+;
  - macFUSE 5.5+;
  - the macFUSE file system extension is enabled;
  - FileVault and iCloud Drive (recommended).

  If anything is missing it shows a "Requirements" sheet with "Download macFUSE" / "Open System Settings" buttons. When everything is fine it shows nothing. The sheet is also available in the padlock menu and in Settings.
- The FSKit extension is checked with a trial mount of an empty volume without a vault (approx. 1 s). The result is remembered per macFUSE version, and every successful vault mount confirms it.
- CLI: new `xordock doctor [--probe-mount] [--json]`. The macFUSE library is loaded only when mounting (`dlopen`), so without macFUSE the CLI works (status, backup, recovery) and clearly refuses only mounting.
- Tests: Go `TestProbeFS`, `TestDoctor`; Swift `requirementsRules` (17 in total). E2E: probe and mount of a vault on a USB stick with the dynamically loaded library.

### S010: tutorial

- On every start the app asks "Start the tutorial?" and offers a "Don't ask at launch" checkbox. The setting can be changed in Settings (⌘, or the padlock menu), where the tutorial can also be started at any time.
- Introduction: 6 screens with a "Next" button:
  - the problem;
  - two halves;
  - A on the Mac, B on the card;
  - copies (spare card, iCloud);
  - five rules;
  - honest limits of the protection.
- Guidance on a real vault: callouts next to buttons, a step bar at the bottom of the window, hints in sheets. The steps "create a vault", "open", "unmount" and "back up" complete on their own when you perform the action. The tutorial never does anything on the user's behalf.
- Swift tests: 3 new (state machine, text length, no commands in the tutorial code), 16 in total. Screen renders in `docs/ui/`.

### S009: clear vault dashboard in the app

- The vault view is a dashboard instead of a form:
  - one status with one recommendation: "All good / Open vault", "No iCloud copy of A / Back up now", "Connect the working card"…;
  - four media tiles (This Mac/A, working card/B, spare card/copy of B, iCloud/copy of A) with ✓/⚠︎ and a date;
  - a row of secondary actions; technical details collapsed.
- Status dots in the vault list, short messages after actions (full output under "Details"), Polish dates.
- Rules in `XordockCore/Health.swift` (7 states with priorities), the view in the new `XordockUI` library, the `XordockPreview` tool rendering the dashboard to PNG (examples in `docs/ui/`).
- Swift tests: 3 new (status priorities, tiles, text length), 13 in total.

### S008: share A copy in iCloud Drive

- Backup writes one recovery point in two places: the snapshot's B shares to the spare card and the same snapshot's A shares to `iCloud Drive/xordock/<name>`. Losing the SSD no longer means losing data.
- Engine:
  - `BackupPair`, generalized side copying (`copySide`), `PruneACopy`;
  - `OpenRecovery` from the A copy, with the spare or working card;
  - `ReadCopyID`;
  - the A copy target must not lie in the share directory or on the card.
- CLI:
  - `backup` makes the A copy by default (`--no-icloud` skips it), `backup prune` prunes both targets;
  - `recover --from-icloud`, `points --from-icloud`;
  - `status` and `vaults` show the A copy, and `orphanCopies` lists copies with no A share on the Mac.
- App:
  - one-time warning about the iCloud account (2FA, Advanced Data Protection);
  - A copy state and a warning when it is missing;
  - recovery from iCloud without the SSD.
- Tests:
  - Go: 6 new (SSD loss, no B and no plaintext in the copy, refusal of locations on the card, interruption at 3 stages, retention of both copies, CLI state);
  - Swift: 1 new, 10 in total.
- E2E: USB sticks + real iCloud Drive. File recovered byte for byte after deleting the A directory, with both the spare card and the working card. Test vault deleted.
- Analysis: `docs/security/2026-11-22-round-S008.md`. R8-01 (by design): A copy in iCloud + any B card = a complete set, so after losing a card all copies of the old A are deleted.
- `docs/recovery.md`: "what is lost → where you recover from" table, procedure after SSD loss, warning against exporting to Desktop and Documents synced with iCloud.

### S007: macOS app (menu bar + window)

- `macos/`: SwiftUI app with a menu bar icon and a window:
  - vault list with card, mount and backup state;
  - new vault wizard with media chosen from a list;
  - mount and unmount, "Show in Finder";
  - "Back up now" (unmount with consent, snapshot, backup to the chosen spare card);
  - recovery: list of points, verification, plaintext export, restore to a new card;
  - `fsck` and the mount log.
- The engine stays single: the app runs the bundled CLI via `Process` with an argument array, no shell. The mount process logs to `~/Library/Logs/xordock/<name>.log` and survives the app being closed.
- CLI: `vaults`, `volumes` and `points` commands with JSON output. `vault.Peek` reads the state of a mounted vault too, without a lock and without changes to the shares.
- `scripts/build-app.sh`: assembles `build/xordock.app` with the CLI in `Contents/Resources`, ad hoc signature.
- Security:
  - vault name validation in the CLI and UI (R7-01): previously `--name ../x` led the A, B and mount point paths outside their directories;
  - the bundled app does not accept a CLI substitution via `XORDOCK_CLI` (R7-03).
- Tests:
  - Go: `TestPeekWhileOpen`, `TestCollectVaults`, `TestWholeDisk`, `TestValidName`;
  - Swift: 9 tests, including the JSON contract, the 7-day rule, command arguments, a ban on the shell and on share access in UI code, the real CLI.
- S006 (A and B on different physical media) recorded as a draft with the owner's decisions. Gap confirmed: a `.dmg` image on the SSD passed as a card.
- Analysis: `docs/security/2026-11-22-round-S007.md`.

### S005: fixes from the S004 round security analysis

- `backup prune` checks both shares of every object of the newest point before deleting anything older (R4-01). Test `TestPruneRefusesIncompleteNewest`.
- The index read from the shares is validated (canonical paths, parent directory), and export refuses paths outside the target directory (R4-02). Test `TestIndexPathsValidated`.
- Analysis: `docs/security/2026-11-22-round-S004.md`. F-01 and F-02 from round S001 closed. Open R4-03 (medium, mitigated): a commit error on `close` does not reach the application through FSKit.

### S004: folder in Finder via macFUSE/FSKit, snapshots, backup and recovery

- The owner's spec recorded as S004. The core stays in Go (owner's decision, deviation from ADR-001), decisions A-01–A-12 in S004 §9. S001 superseded, S002 abandoned; the uncommitted `macos/` project with a custom FSKit module removed.
- `internal/split`: masks from `SecRandomCopyBytes` (cgo, result code checked), description "practical protection, no information-theoretic guarantee"; deterministic TEST_ONLY source in tests only.
- `internal/vault`:
  - Snapshots in the XOR-split index (`CreateSnapshot`, `Snapshots`, `DeleteSnapshot`, `SnapshotView`, `Changes`). GC respects snapshots and pauses when a snapshot index is unreadable.
  - Backup to a spare card (`backup` role in the header): pinning, copying missing B shares, verification, READY point. Free space check before any change, `PruneBackup`.
  - Recovery (`OpenRecovery`, `Verify`, `View`), `ExportTo`, `CopyInto` (new vault, new masks). `FindCards`, `fsck` covers snapshots. A spare card is rejected as a working card.
- `internal/fusefs`: file system on top of the engine: write buffers in RAM, spill of full blocks above 64 MiB, commit on flush/fsync/release, truncate, rename (EXCL; SWAP → ENOTSUP), unavailable state after card removal with buffers wiped.
- `internal/fusemount`: cgo bridge to libfuse3 from macFUSE (`backend=fskit`), card supervision every 1 s with forced unmount, detection of a failed mount. `chmod`/`chown` have no effect (A-06).
- CLI: `mount`, `unmount`, `status`, `snapshot create|list|rm`, `backup [prune]`, `recover list|verify|export|restore`; cards recognized by vault identifier and role.
- Tests: 20 new, 44 in total plus `TestPerf` (snapshots, GC, backup with failures at 3 stages, out of space, retention, recovery, CG-09, no plaintext on the spare card, adapter: random-write model, spill, directories, truncate, card removal, deleting an open file; TEST_ONLY mask source and generator failure). Two of them (S005) are added in a separate entry above.
- E2E (macOS 27.0, macFUSE 5.5.0, HFS+ SD card, disk images as the spare card and the new card):
  - mount in Finder, mkdir, write and `cmp` of 5 MB and 30 MiB, append, rename between directories, 30 small files, delete, `truncate`;
  - no marker and no names in A, B, on the spare card, in the new vault, `/private/tmp` and `/private/var/folders`;
  - simulated card removal: unmount after ≤ 1.25 s;
  - `fsck`, snapshot, backup, change after backup, `recover verify/export/restore` without the working card: exact bytes of the point, without the later change;
  - test vaults deleted from the card.
- Fixed during E2E: `chmod` returning ENOTSUP blocked `mkdir`/`create` through FSKit; the CLI reported mount and unmount despite a failed mount or an emergency unmount.
- Documentation: `docs/format-v1.md`, `docs/recovery.md`, update of `docs/threat-model.md`, `docs/usage.md`, `MANIFEST.md` (stack).

### S002 step 1: FSKit trial

- `macos/`: Xcode project (container app `pl.indatify.xordock` + FSKit extension `pl.indatify.xordock.fs`), read-only "hello" module. It compiles (Xcode 27.2, macOS SDK 27.2).
- Signing: Apple Development certificate and the intermediate WWDR G3 installed by the owner.
- **Blocker:** Personal Team does not support the FSKit Module entitlement, so Xcode does not create a profile. Further work requires the paid Apple Developer Program.

### S002 (preparation): WebDAV removal, FSKit direction

- The owner rejected WebDAV and NFS; Finder integration is to be native (FSKit). Removed `xordock mount`, `internal/davfs`, `internal/mount` and the `golang.org/x/net` dependency. The project has no external dependencies.
- F-01 and F-02 from the round S001 analysis no longer apply to the code (integration removed). They will be rechecked in S002.
- A trial NFSv3 server was written and abandoned before the first successful mount (it did not make it into the repository).
- `internal/vault`: stable file numbers in the index (`Entry.Ino`, root directory = 1, filled in for old indexes), operations by number and with file system semantics: `ByIno`, `SetFileIno`, `SetModTime`, `Create`, `Remove` (unlink/rmdir), `RenameReplacing`. Tests: `TestInodesStable`, `TestReplaceAndRemoveSemantics`, `TestEnsureInosForOldIndex`.
- S002 rewritten for FSKit. Setup: Xcode 27.2 with the FSKit SDK; no signing identity.

### S001: xordock Secure Folders prototype

- Spec S001 (owner's draft, working name OpenTrust → xordock). Decisions Q-01–Q-06 and FR-05 per the recommendations, at the owner's instruction (S001 §9.8).
- Go, module `xordock`, a single dependency `golang.org/x/net/webdav`.
- `internal/split`: XOR split, CSPRNG mask source with an explicit statement of no P = NP guarantee (CG-10).
- `internal/vault`: share format v1 (magic, side, vault, object, length, SHA-256 of its own share), 1 MiB blocks, every write is a new object with a new mask, index with file names split with XOR. Commit: objects (fsync) → F_FULLFSYNC barrier → superblock A → superblock B. Recovery: completing or rolling back an interrupted commit, other states are an explicit error. GC of old versions (B first). `fsck` without reassembling content. `flock` lock.
- `internal/davfs`: WebDAV file system on top of the vault: writes at arbitrary offsets, holes, spill of full blocks on sequential writes, commit on close.
- `internal/mount`: server on 127.0.0.1 with a random token in the path, rejection of foreign `Host` and `Origin`, `mount_webdav`, card supervision every 1 s (error state + forced unmount).
- CLI `xordock init|mount|fsck|export`. `mount` requires FileVault.
- Tests: 27 unit and protocol tests (plus `TestPerf` run manually), including 13 failure scenarios at every write stage. E2E on a real SD card (HFS+) and SSD: write/read of 20 MB and 3 MB, rename, delete, 50 small files, remount, simulated card removal, lock during a concurrent `fsck`, export, no content marker and no file name in either share.
- Performance: first version (64 KiB block) 1.4 MiB/s write, after switching to 1 MiB and syncing directories once per commit 10.4 MiB/s. Read ~600 MiB/s.
- Round security analysis: `docs/security/2026-11-22-round-S001.md`. No critical regressions. F-01 (WebDAV agent cache on the SSD, high) open → S002. F-02 (token visible in `mount`) accepted for the prototype.
- Documentation: `docs/threat-model.md`, `docs/usage.md`. Drafts S002 (integration without cache) and S003 (protecting a folder from Finder, owner's request).

### Project start

- `git init` (branch `main`), CDD artifacts (`CLAUDE.md`, `README.md`, `CHANGELOG.md`, `MANIFEST.md`, `specs/SZABLON.md`), `.gitignore`. Empty `tests/` directory removed in S001: Go tests live next to their packages.
