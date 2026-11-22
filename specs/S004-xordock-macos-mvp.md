# S004 — XorDock: secure macOS folder with two shares and recovery

- **Status:** delivered for CP-01–CP-08 (CP-08: SwiftUI app in S007). Open part of CP-09 (fuzz, 1 GiB nightly test, 1 GiB measurement). Details in §9 "Implementation".
- **Date:** 2026-11-22
- **Related:** supersedes S001 (engine taken over) and S002 (abandoned); S005 (fixes from this round's security analysis); S003 (draft)
- **Source:** owner's spec entered as `specs/S004.md` (working title SPEC-001). Number and file name follow the repository convention. The owner's text below is unchanged in content (translated from Polish to English by the owner's decision); implementation decisions added in §9.

> Owner's text:


**Version:** 1.0-draft  
**Date:** 2026-11-22  
**Methodology:** Rondo / Context-Driven Development (CDD) — https://workwithrondo.com/  
**Status:** Ready to start *prototype implementation*, not for storing production data  
**Recipient:** independent author of the XorDock product  
**Repository:** `xordock` (new)  
**Target file:** `specs/SPEC-001-xordock-macos-mvp.md`

## 1. TL;DR

Build on macOS a folder visible in Finder, each block of whose content and sensitive metadata is split into two XOR shares: `A` on the Mac's internal SSD and `B` on a working SD card. Add snapshots of share A versions and synchronization of the corresponding B blocks to a second SD card, so that after losing the working card it is possible to recover the *last valid recovery point*, but not later changes. Success must be confirmed by automated tests of file operations, medium loss, simulated power failures and full recovery.

## 2. Abstract

The project starts without an existing implementation. Ordinary disk encryption with a short key and an optional token does not meet the considered goal of physically separating information: a copy of a single share alone should not be enough to restore the documents.

XorDock implements a simple data split: for a message `M` it generates a mask `R`, writes `A = M XOR R` on the SSD and `B = R` on a removable medium. Reconstruction `M = A XOR B` happens only with access to both shares. For convenience the system presents an ordinary volume/folder to the macOS user.

In the MVP the masks come from the macOS system cryptographic generator (CSPRNG). This provides a strong *practical* security model, but **is not a proof of perfect secrecy nor of unconditional resistance to P = NP**. A variant with a verified stream of independent random masks requires a separate spec and a cryptographic review.

Two identical media are not RAID 1. The working SD holds the active share B. The spare SD stores the **historical share B** for the last committed recovery point. After losing the working SD only that historical version can be restored, using a preserved, matching snapshot of share A. We do not use snapshots of the whole APFS file system.

The first release is a local prototype on the owner's own Mac, distributed outside the Mac App Store. By default it must not require disabling SIP/AMFI or lowering the Apple Silicon security policy.

## 3. Objective

### Goals

- **G-01:** Create and mount in Finder one protected `XorDock` volume, independent of the system disk.
- **G-02:** Store file contents and sensitive metadata only as two matching shares, with no persistent plaintext files in XorDock stores.
- **G-03:** Do not mount the volume if a valid share B is missing; do not replace the missing share with zeros, old blocks or CSPRNG output.
- **G-04:** Provide ordinary file operations and durable, consistent writes despite two physical media.
- **G-05:** Keep historical versions of A blocks only for XorDock snapshots.
- **G-06:** Synchronize the spare SD once per user request (e.g. weekly), copying only the missing B blocks of the recovery point.
- **G-07:** After losing the working SD, restore a consistent version of the folder from the last committed A snapshot and the corresponding spare SD.
- **G-08:** Provide diagnostics: whether the volume is mounted, which medium is required, when the last verified backup was made and which changes may be lost.
- **G-09:** Verifiably distinguish the confidentiality of theoretical XOR with an ideal R from the real protection of an implementation using the macOS CSPRNG.

### Out of scope — MVP

- Booting from the protected volume; encrypting the system SSD, replacing FileVault.
- iPhone as a backend, SMB/NAS, synchronization over the Internet, iCloud and data sharing.
- A custom FSKit extension signed with an individual entitlement.
- Software erasure of data from flash cells as a guaranteed security feature.
- A guarantee of protection against a compromised system, a malicious application or memory snooping of a running Mac.
- A guarantee of integrity against an adversary with arbitrary computing power; it must be designed separately.
- Full information-theoretic security of the product under P = NP: it is a research requirement after the MVP, not a claim of a finished product.
- Full compatibility with every macOS application and all POSIX features in the first release.

### Success criteria

**SC-01:** files after writing and remounting are byte-identical;  
**SC-02:** neither share A alone nor share B alone contains plaintext test documents;  
**SC-03:** disconnecting the working SD ends with a deterministic operation error and a safe state;  
**SC-04:** after losing the working SD the last committed backup point can be restored, not later changes;  
**SC-05:** no simulated interruption of a transaction silently creates a damaged "committed" version;  
**SC-06:** SIP-off, AMFI-off, Reduced Security or a paid Apple account are not needed for the local prototype;  
**SC-07:** the documentation contains no unjustified promise of "resistance to P = NP".

## 4. Artefact Inventory

| Artefact | State | Function |
|---|---|---|
| `README.md` | to create | Current actual state, installation, limitations |
| `CHANGELOG.md` | to create | What was actually built and tested |
| `specs/SPEC-001-xordock-macos-mvp.md` | to create | This implementation contract |
| `docs/engineering-manifesto.md` | to create | Rules for quality, security, committing and reviews |
| `docs/threat-model.md` | to create | Exact threat and limitation model |
| `docs/format-v1.md` | to create | Format of shares, versions, catalog, transactions and manifests |
| `docs/recovery.md` | to create | Backup procedure and recovery after losing the SD |
| `core/` (Rust) | to create | XOR, objects, manifests, transactions, snapshots and recovery |
| `macos/` (SwiftUI) | to create | Interface, SD selection, mount/unmount, diagnostics |
| `fuse-adapter/` | to create | Adapter of FUSE operations to `core/` |
| `tests/unit/` | to create | Tests of operations, masks, manifests |
| `tests/integration/` | to create | Two stores, transaction consistency, hot-unplug |
| `tests/recovery/` | to create | Recovery tests from a historical point |
| `scripts/fault-injection/` | to create | Controlled interruption of write/commit/backup |
| macFUSE 5.x + FSKit backend | exists (external) | FUSE without its own kext, confirmation required on the target macOS version |
| `SecRandomCopyBytes` | exists (system) | CSPRNG for the prototype; not unconditional randomness |
| Test SSD/SD and spare medium | to provide | Physical test of the full scenario |
| Public API for raw hardware TRNG on macOS | not established | Do not assume it as an existing dependency |

**Platform decision:** Apple Silicon, macOS 26 or newer. When coding starts, record the exact versions of macOS, Xcode and macFUSE in `README.md` and CI. Use supported APIs. In the production phase check the macFUSE license — commercial bundling or automatic installation of the binary distribution requires separate consent/license.

## 5. Functional Domain Changes

### 5.1. Data and security model

**D-01.** For each new logical block `M` the application requests an independent mask `R` with the identical number of bytes. `A = M XOR R`, `B = R`. Recovery requires both shares with the same identifier and the same version.

**D-02.** Blocks are *immutable*: rewriting a given file fragment creates a new object identifier, a new mask and two new shares. It is forbidden to overwrite a share while keeping the earlier mask, or to use the same mask for two versions.

**D-03.** Initial block size: 1 MiB (a format parameter, with tests for small and large files). The final block may be shorter; sizes remain observable metadata. Format evolution requires versioning.

**D-04.** File names, the folder tree, text attributes and the file-to-block mapping are part of the protected logical catalog — we split them into shares just like the content. Plaintext medium headers may contain only the minimum: volume identifier, share role, format version, generation number and opaque object identifiers. Disclosure of the number, sizes and rhythm of changes may remain possible; we document this.

**D-05.** No field in share A or share B may contain the plaintext original file or its name. Do not use plaintext hashes as object names. Simple checksums *of the share itself* serve to detect accidental damage, but do not confirm integrity under active attack.

**D-06.** `RandomSource` has the interface `fill(buffer) -> Result`. The MVP implements an adapter for the system `SecRandomCopyBytes`, checking the result code; a generator error stops the write. Tests use a deterministic adapter explicitly marked `TEST_ONLY`, which must not end up in a release build. The interface for a future source of physical randomness is an extension, **not** a promise of an achieved guarantee.

**D-07.** Preventing plaintext leaks: the engine does not write plaintext to working folders, logs, crash reports or the backup layer. It cannot be guaranteed that external applications will not leave plaintext in temp/cache/Quick Look/swap/Time Machine. The program shows a warning and recommends FileVault.

### 5.2. Interface and mounting

**F-01.** Volume creation: the user selects the folder for store A on the internal disk and prepares a working SD card as store B. Initialization generates a pair identifier unrelated to the volume name. Do not assume that an ordinary SD card is an uncopyable token.

**F-02.** Mounting happens only after checking a consistent pair of shares and a common committed generation. When one side is newer due to a failure, the system chooses the last *jointly* committed generation or refuses to mount with repair information.

**F-03.** Standard MVP operations: listing, creating files/folders, reading a fragment, writing a fragment, resizing, rename, delete, `stat`, `flush`, `fsync` and reopening. Support for `xattr`, aliases/symlinks, locks and application atomic saves requires separate compatibility tests. Unsupported operations must return an explicit error, not an apparent success.

**F-04.** The virtual folder appears in Finder. Disconnecting the working SD switches the store to the `DEGRADED/READ_ONLY_OR_UNAVAILABLE` state and refuses new operations requiring B. It is forbidden to buffer the whole share B on the SSD or keep it persistently in a cache.

**F-05.** `unmount` stops new opens, synchronizes active operations, completes transactions, marks the volume as closed and allows the card to be ejected safely. Do not promise removal of plaintext data that applications already hold in their memory.

### 5.3. Consistent writing to two media

**T-01.** File change: write new A and B blocks under new IDs; persist the objects (`fsync`); prepare a new protected metadata catalog; persist both sides; write a `prepared` record on both media; move to a common `committed` and update the volume root. The format must be resilient to the lack of atomicity between disks.

**T-02.** After a restart the scanner chooses the last common valid `committed` root, checks the completeness of the referenced objects and discards unfinished objects. `fsync` in the visible file system must not confirm data durability until the required shares have been persisted.

**T-03.** Concurrent writes must be serialized within the scope of the modified objects and the manifest root. The first prototype may have one writer per volume, if the interface clearly enforces the restriction.

**T-04.** Out of space, missing SD, I/O error, power cut and manifest damage: no silent loss of the last confirmed version; an error and recovery instructions.

### 5.4. Snapshots of XorDock shares only

**S-01.** A snapshot is an immutable identifier of a committed XorDock catalog root: `snapshot_id`, `generation`, the range of required block identifiers and a timestamp. The associated logical catalog state remains split into A/B.

**S-02.** `createSnapshot()` does not take a snapshot of the whole APFS volume and does not copy plaintext. It pins historical A and B objects so that overwriting a document does not delete blocks needed by an older snapshot.

**S-03.** Changing a file after a snapshot creates new objects and a new logical catalog, and the old objects remain as long as a snapshot or backup requiring them exists.

**S-04.** `garbageCollect()` deletes only objects unreachable from the current root and all pinned snapshots/valid backups. When in doubt — it stops.

### 5.5. Weekly backup to the second SD

**B-01.** Workflow: insert the working SD, create snapshot `G`; insert the spare SD; XorDock copies the *missing* share B blocks for `G` and the corresponding versioned share B metadata. The required share A blocks remain pinned locally.

**B-02.** The backup has a separate medium identifier and recovery point identifier, but is not a separate cryptographic key. It copies share B in the same version. Copying B to the second SD is not re-encryption with the same OTP; it is a copy of the same share.

**B-03.** A new backup is created in a staging area. The system confirms it as `READY` only after checking the presence and validity of all required objects. **In case of error, the previous verified recovery point must remain complete.**

**B-04.** If the spare SD lacks space for safe staging, synchronization refuses to run instead of deleting the only good backup. Deleting an old version requires a separate retention decision by the user and a new, successfully completed point.

**B-05.** On completion the application shows the date and ID of the `READY` point, the scope of guaranteed recovery and a warning that later changes are **not** on the spare card.

**B-06.** Backup of A on a separate physical medium is a future feature. The local A snapshot + spare SD protect against loss of the **working SD**, but not against failure or loss of the Mac's SSD.

### 5.6. Recovery

**R-01.** The user inserts the spare SD after losing the working one. XorDock shows matching share A snapshots and B points marked as `READY`.

**R-02.** `restoreSnapshot(G)` requires an exactly matching set of A and B objects of the same generation `G`; it does not mix `A_G` with `B_(G+N)` and does not "fill in" with guessed data.

**R-03.** Recovery performs a full read and completeness check into a new set of shares or into a specified plaintext export folder. It must not modify or delete the only verified backup copy.

**R-04.** After recovery the user can create a new working SD from a copy of share B, and then continue working in a new generation. Treat the lost card as potentially copied — do not promise its remote revocation.

### 5.7. Integrity, confidentiality and threats

- **I-01:** The main information-theoretic assurance concerns only the *confidentiality* of a single perfectly random share, not availability, integrity, host security or proof of data provenance.
- **I-02:** For the MVP check accidental integrity: checksums of each part computed over that same part, consistency of ID, size, version and roots. A checksum alone does not protect against deliberate share substitution. A mechanism for authenticity against an attacker modifying the medium requires a separate cryptographic decision, which must not undermine the single-share property.
- **I-03:** Do not place on a single share plaintext markers, hashes or MACs computed from the **plaintext** that could reveal information about the file.
- **I-04:** The program must document the risk of leaks through RAM, swap, Quick Look, Spotlight indexing, application temporary saves, crash dumps, Time Machine copies, iCloud backup and installed extensions. Features limiting such leaks are an additional stage, not a default assumption.
- **I-05:** Removing the working SD when all data is already in plaintext in an application does not remove it from the Mac's memory. Do not guarantee resistance to malware or an administrative user.
- **I-06:** File names and sizes in the active folder may appear outside the share store as a result of system and application activity — the mandatory threat model must account for this.

## 6. Critical Path

1. **CP-01 / Project start:** create the repository, README, manifest, this spec, tests and a baseline threat model. Do not design the UI before the data model is settled.
2. **CP-02 / Minimal engine:** test CLI `init`, `put`, `get` for two folders in separate test locations; CSPRNG mask, XOR, explicit errors, byte tests.
3. **CP-03 / Immutable object store:** A/B objects + a versioned split logical catalog, new masks on changes, no overwriting of versions.
4. **CP-04 / Transactions:** persisting both shares, a common root, recovery after an interrupted write and reference-based garbage collection.
5. **CP-05 / Snapshots:** `snapshot create/list/pin`, generations, immutable historical A/B blocks, no APFS-volume snapshot.
6. **CP-06 / Recovery:** backup to the spare SD, `READY`, test of losing the working SD, restoring the last point and creating a new working SD.
7. **CP-07 / FUSE:** only after a positive spike checking macFUSE 5.5 `backend=fskit` on the target macOS version; a FUSE adapter to a stable Core with no cryptographic logic in the adapter.
8. **CP-08 / UX:** SwiftUI, media selection, mounting, backup/restore, point history and clear messages.
9. **CP-09 / Validation:** failure tests, fuzz, retention, speed measurement, security analysis; only then a demonstration on non-sensitive files.

**End of the critical path:** the user can use test files in Finder, create a valid recovery point, lose the working SD and recover the folder state from that snapshot without manual XOR.

## 7. Control Gates

Each gate gives an unambiguous verification. **Failure of any gate blocks marking the stage as completed**; all tests should run automatically in CI, except explicitly marked hardware tests.

| Gate | PASS condition | Kind of evidence |
|---|---|---|
| CG-01 | `get(put(X)) == X` for 0 B, 1 B, 4 KiB, 1 MiB, 5 MiB, 1 GiB (optional nightly test) and binary files | Unit/integration |
| CG-02 | Every edit creates a different mask ID; no mask is used in two versions of a block | Unit/property |
| CG-03 | Absence of a valid SD prevents mounting; a wrong SD is rejected | Integration/E2E |
| CG-04 | After `fsync` both shares and the manifest survive a clean restart and a restore test | Integration |
| CG-05 | Interruption after every write/commit step ends with the last common committed state or an unambiguous error | Fault injection |
| CG-06 | `createSnapshot()` records a historical root, without a whole-APFS snapshot and without copying all plaintext | Integration |
| CG-07 | `snapshot G1` and a later write `G2` still allow reading the exact `G1` with valid shares | Integration |
| CG-08 | Spare SD `B_G1` + snapshot `A_G1` restore the exact bytes from `G1` | Recovery E2E |
| CG-09 | Spare SD `B_G1` is not wrongly used to restore new blocks from `A_G2` | Negative recovery |
| CG-10 | Interrupting a backup leaves the previous `READY`; it does not mark the interrupted point as `READY` | Fault injection |
| CG-11 | Lack of space for a backup does not erase the last working copy and returns an error | Negative integration |
| CG-12 | No persistently visible plaintext in A/B stores, engine logs and object names, within the defined test | Security inspection |
| CG-13 | No calls requiring SIP/AMFI to be disabled; mount works on the target Mac via the macFUSE FSKit backend | Manual hardware gate |
| CG-14 | `README.md` and the UI describe the CSPRNG as a **practical** guarantee, not an information-theoretic one | Documentation review |
| CG-15 | After each round a threat, a security regression review and a test or justification for the lack of a test are recorded | Security review |
| CG-16 | The macFUSE dependency has verified license terms for the kind of distribution; commercial bundling without consent is forbidden | License review |

**Release prohibitions (negative gates):** the MVP must not be advertised as "P=NP-proof"; the last backup must not be deleted automatically; the application must not be given write access to all media without the user's conscious choice; kext/reduced security must not be enabled just to improve performance; real secret documents must not be used before the failure and recovery gates have passed.

## 8. Expected Outcome

**Direct effect:** A XorDock folder is created on the Mac, working when the working SD card is connected. Files are stored as split shares on the SSD and the SD. The user can create snapshots of the protected folder, synchronize the spare SD and restore the last point after losing the working card.

**Measurable effect:** measure the space growth for one recovery point, the number of changed blocks to synchronize, read/write of small and large files, the time to mount and restore 1 GiB. Do not declare performance thresholds before measuring. For selected tests the restore must yield an identical SHA-256 of the plaintext *in the test tool*, not stored as a public marker on a single share.

**Side effects:** double the volume of active shares required, additional A history, SD latency, risk of medium failure, no availability without the card, no restore of changes after the backup date, risk of plaintext copies outside XorDock.

**Rollback:** The program must provide a deliberate export of plaintext files when both shares are present. It does not delete them automatically nor claim that the export is still protected. After exporting, the user can safely unmount XorDock. Uninstalling the application without exporting does not guarantee continued access to the data.

## 9. Additional Information

### ADR — technical decisions

- **ADR-001:** Rust engine, independent of UI/FSKit/FUSE; the FUSE adapter only translates file operations into the engine API.
- **ADR-002:** SwiftUI for managing the volume and media. The first milestone may be CLI only.
- **ADR-003:** macFUSE >=5.5.0 with `backend=fskit` as the first candidate. Perform a mount test and compatibility tests before committing to the architecture. We do not require our own `com.apple.developer.fskit.fsmodule` entitlement, because we do not create our own FSKit extension.
- **ADR-004:** Apple CSPRNG via `SecRandomCopyBytes` with explicit status checking; do not assume access to the raw TRNG in the Secure Enclave.
- **ADR-005:** Copy-on-write implemented in our own object store and manifests, *not* via APFS volume snapshots. APFS `clonefile()` only as a later optimization.
- **ADR-006:** In the MVP only two physical shares A/B. The spare SD is a copy of B **of a specific version**, not a third Shamir share.
- **ADR-007:** The prototype is independent of the Mac App Store. For commercial integration of macFUSE check the license terms and, if needed, obtain consent/license or change the mounting layer.

### Assumptions and security consequences

- Obtaining one *perfectly random* share is information-theoretically insufficient to restore the data. This does not mean that the output of `SecRandomCopyBytes` has OTP perfect secrecy.
- The SD card is copyable; "physical presence of the card" does not mean that no adversary copied share B earlier.
- Lack of physical separation (e.g. two folders on the same SSD) does not protect against copying the whole SSD.
- The spare B in the vault does not protect against failure of the Mac's SSD; that requires a separate backup of the A snapshot, stored independently.
- Perfect secrecy of 100 GB of arbitrary data requires sufficiently long full-entropy material for each protection version; statistical tests of a pseudorandom stream do not prove this property.
- A variant for P = NP requires a separate spec for validating the true randomness source, protecting version history, integrity and a comprehensive leak audit.

### Rejected alternatives

1. Full-disk XorDock — rejected for the first version because of boot, compatibility and difficult recovery.
2. Whole-APFS snapshots — rejected, because we want to version only the protected share and not interfere with the whole volume.
3. An ordinary AES key file on the SD — does not achieve physical splitting of information.
4. A Shamir 2-of-3 key for short secrets only — does not provide perfect secrecy of the files themselves under P = NP.
5. iPhone/NAS as an active backend — after the MVP; requires an additional transport design and an eavesdropping model.
6. Our own native FSKit from day one — unnecessary for the prototype and increases signing/entitlement burdens.

### Open issues blocking a production product, not the prototype

- Exact handling of Finder/Quick Look consistency, `xattr`, `mmap`, Office autosave, SQLite-on-FUSE and applications creating temporary files.
- Direct integration with FSKit instead of macFUSE, if the license, performance or distribution model requires it.
- Level of authenticity and integrity against malicious modification of a share.
- Retention rules, backup of A, details of restoring after losing both media.
- An external source of physical randomness and validation of the full-entropy amount of data.
- An independent audit of security and of the quality of the randomness source.

### Context and references

- Rondo: https://workwithrondo.com/ — nine sections, artifacts `specs/`, `README`, `CHANGELOG`, tests and a security review after changes.
- macFUSE: https://macfuse.github.io/ and https://github.com/macfuse/macfuse/wiki/FUSE-Backends — FSKit backend, `backend=fskit`, API limitations.
- macFUSE License: https://github.com/macfuse/macfuse/blob/release/macfuse/LICENSE.txt — restrictions on commercial bundling of binaries.
- Apple SecRandomCopyBytes: https://developer.apple.com/documentation/security/secrandomcopybytes(_:_:_:)
- Apple Platform Security: https://support.apple.com/guide/security/random-number-generation-seca0c73a75b/web

### Rondo / CDD process

For each subsequent feature create a new file `specs/SPEC-00N-*.md` with nine sections. After implementation update `CHANGELOG.md` only with what was actually completed, and `README.md` according to the state of the code. Tests and security gates are to be the proof of delivering the change. Do not close a spec if a critical gap has the status "to be fixed later".


### Implementation (2026-11-22, added by the implementer)

#### Owner's decisions and deviations

| ID | Decision | Justification |
|---|---|---|
| A-01 | **Core in Go, not in Rust** (deviation from ADR-001). Owner's decision 2026-11-22 after the implementer's question. | The S001 engine (shares, A→B commit, crash recovery, GC) was tested; a rewrite would produce a second implementation of the format (MANIFEST M2). Rust from Homebrew was also broken on the workstation. Go is memory-safe like Rust; the FUSE adapter contains no cryptography (ADR-001 preserved as to the division of responsibility). |
| A-02 | The project does **not** start from scratch (§2 says otherwise): S004 takes over the S001 engine and extends it. We map the §4 inventory to existing artefacts: `core/` → `internal/split`, `internal/vault`; `fuse-adapter/` → `internal/fusefs` (logic) + `internal/fusemount` (cgo bridge to libfuse3); `tests/*` → `*_test.go` next to the packages; `scripts/fault-injection/` → `Options.Hook` hooks in tests; `docs/engineering-manifesto.md` → the existing `MANIFEST.md`. | Consistency with the repository convention (Go tests next to packages, one manifest). |
| A-03 | Masks from `SecRandomCopyBytes` via cgo, result code checked (D-06 1:1). The deterministic TEST_ONLY source lives only in a `_test.go` file, so it cannot end up in the binary. | D-06, ADR-004. |
| A-04 | Snapshots recorded in the index (XOR-split), not in a plaintext file. Every change of the snapshot list is an ordinary A→B commit. | D-04 (protected metadata), T-01 (same consistency protocol). |
| A-05 | T-01 "prepared/committed": implemented by the existing protocol: objects + fsync, an `F_FULLFSYNC` barrier on both media, superblock A (prepared state), superblock B (committed). On open: complete or roll back; other states are an explicit error (F-02, T-02). | No format change; covered by `TestCrashAtEveryStage`. |
| A-06 | `chmod`/`chown` accepted without effect (deviation from F-03 "explicit error"). Permissions are not stored: files 0600, folders 0700, owner is the mounting user. | The macFUSE FSKit backend sets the mode on every `mkdir`/`create`; an error from `chmod` blocked file creation (E2E 2026-11-22). |
| A-07 | `rename` with the swap flag (`RENAME_SWAP`) returns `ENOTSUP`; `RENAME_EXCL` supported. `xattr`, symlinks, hard links: unsupported (ENOSYS/ENOTSUP). | F-03: no apparent success. |
| A-08 | Backup requires the working card and the spare card **at the same time** (the second via a USB reader). B-01 describes swapping cards in one slot, but B shares must not be buffered on the SSD (F-04, G-02), and a RAM buffer for a large snapshot is impractical. | Security over convenience; to be revisited in CP-08. |
| A-09 | Recovery (`recover restore`) creates a **new vault with new masks** on a new card, instead of copying share B to a new working card (R-04). | The lost card may have been copied (R-04). With a copy of B, its masks would match the restored A shares. The old share A and the spare card remain untouched (R-03); the program recommends deleting the old A after verification. |
| A-10 | Retention: `backup prune` keeps only the newest READY point and removes the pins of older ones; a snapshot pinned by a point can be deleted only after prune. Nothing happens automatically (B-04). | Explicit user decision. |
| A-11 | Snapshots and backup only with the vault unmounted (the CLI takes the same `flock` lock as mounting). | T-03: one writer per volume. |
| A-12 | macFUSE 5.5.0 installed by the owner for private use (prototype). CG-16: BSD license with clause 4: bundling the macFUSE binary with commercial software (including automatic installation) requires the author's written consent. xordock does **not** bundle or install macFUSE. | ADR-007. Before commercial distribution: consent or a change of the mounting layer. |

#### Environment (CP-01)

macOS 27.0 (26A428), Apple Silicon, Go 1.27.4, Xcode 27.2 (cgo/clang), macFUSE 5.5.0 (libfuse3 3.18.3, FSKit backend, extension enabled in System Settings). Working card: HFS+ SD in the MacBook Pro reader.

#### Gates and evidence

| Gate | Evidence | Result |
|---|---|---|
| CG-01 | `TestRoundTripAfterReopen` (0 B…3 MiB+5), `TestRandomWritesMatchModel` (300 random writes/truncations through the adapter), `TestLargeFileSpills`; E2E: 5 MB, 30 MiB of random data, `cmp` after mounting | ✅ (1 GiB nightly: open, CP-09) |
| CG-02 | `TestEveryWriteGetsFreshMask`, `TestTestOnlySourceDeterministic`, `TestMaskErrorStopsSplit` | ✅ |
| CG-03 | `TestMissingCardBlocksOpen`, `TestWrongCardRejected`, `TestRecoverAfterLosingWorkingCard` (spare card rejected as working card) | ✅ |
| CG-04 | `TestRoundTripAfterReopen`, `TestRandomWritesMatchModel` (reopen + fsck); E2E: fsck after unmount | ✅ (see R4-03: fsync via FSKit) |
| CG-05 | `TestCrashAtEveryStage` (13 scenarios), `TestTornTempFilesCleaned`, `TestInterruptedInitFinished` | ✅ |
| CG-06 | `TestSnapshotKeepsOldVersion` (a snapshot adds only the index object) | ✅ |
| CG-07 | `TestSnapshotKeepsOldVersion` | ✅ |
| CG-08 | `TestRecoverAfterLosingWorkingCard`; E2E: backup to a second medium, `recover verify/export/restore` without the working card, `cmp` 30 MiB | ✅ |
| CG-09 | `TestSpareNotUsedForNewerObjects`, `TestRecoverAfterLosingWorkingCard` | ✅ |
| CG-10 | `TestInterruptedBackupKeepsPreviousPoint` (3 stages) | ✅ |
| CG-11 | `TestBackupNoSpace` | ✅ |
| CG-12 | `TestSharesHoldNoPlaintext`, `TestSpareHoldsNoPlaintext`, `TestLogsHoldNoSecrets`; E2E: no marker or names in A, B, the spare card, the new vault, `/private/tmp`, `/private/var/folders` | ✅ |
| CG-13 | E2E 2026-11-22: `mount` with `backend=fskit`, no kext, SIP/AMFI enabled, no paid Apple account | ✅ (manual) |
| CG-14 | `TestKindIsHonest`; CLI messages, README | ✅ |
| CG-15 | `docs/security/2026-11-22-round-S004.md` | ✅ |
| CG-16 | A-12 | ✅ (private prototype) |

#### Measurements (E2E, HFS+ SD card, SSD with FileVault)

Write via Finder/FSKit: 30 MiB in 2.9 s (~10 MiB/s, like the engine in S001). Small file (create + write + close): ~220 ms. Detecting card removal and unmounting: ≤ 1.25 s. Snapshot: 1 object (index). Backup of 30 MiB (33 objects) to a disk image: < 2 s.
