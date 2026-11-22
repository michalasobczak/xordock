# S001 — xordock Secure Folders for macOS

- **Status:** superseded by S004 (2026-11-22). The engine (`internal/split`, `internal/vault`) moved to S004. The WebDAV integration was removed 2026-11-22; §9.8–9.9 describe the state as of commit `98e5fdc`.
- **Date:** 2026-11-22
- **Project:** xordock
- **Platform:** macOS, Apple Silicon
- **Type:** new application / virtual file system
- **Scope:** first usable prototype of a folder based on two data shares
- **Related:** S002 (integration without on-disk cache)

## 1. TL;DR

I want to build a macOS application that gives the user an ordinary folder in Finder, but stores its contents as two independent shares: one on the Mac's internal SSD, the other on a removable SD card.

Without the SD card, restoring the folder contents must not be possible from the share on the Mac alone. The prototype is to demonstrate correct operation and failure handling, while obtaining a confidentiality guarantee independent of P = NP requires a separate, documented validation of the randomness source and of the whole security model.

## 2. Abstract

Current file protection solutions on macOS rely primarily on encryption based on short cryptographic keys. A physical token can control access to such a key, but it does not guarantee mathematical confidentiality of the data if a practical method of breaking the encryption algorithm is found.

xordock is to implement a different model: the contents of each protected file are split into two shares using the XOR operation and a fresh random mask. The first share is stored on the Mac and the second on the SD card. Both are required to restore the data.

The user should not split or join files manually. They should use an ordinary folder visible in Finder.

The project covers selected folders, not the whole system disk. It does not require the SD card to be connected to boot macOS.

The existing projects SafeFS and Lecithine were considered, but it was not assumed that their implementations meet production security requirements.

The biggest risks are the quality of randomness, consistency of writes across two media, applications leaving plaintext copies of files, and the reliability of SD cards.

## 3. Objective

### Goals

**G-01 — Virtual folder.** The system is to present the protected folder in macOS as an ordinary location accessible from Finder and standard applications.

**G-02 — Two physical shares.** Each block of protected data is to be represented by two shares:

- A — on the Mac's local disk;
- B — on the removable SD card.

The following relation holds:

```
A = M XOR R
B = R
M = A XOR B
```

where `M` is the original data and `R` is the random mask.

**G-03 — SD card required.** Without the correct SD card the protected folder cannot be mounted or restored from share A alone.

**G-04 — Normal user work.** The user is to be able to create, read, modify, move and delete files without managing shares manually.

**G-05 — Failure resilience.** An interrupted write must not cause silent restoration of incorrect data or an inconsistent folder without an error message.

**G-06 — Explicitly defined security.** The product must distinguish:

- the prototype using a cryptographically secure pseudorandom generator (CSPRNG);
- the target variant, requiring full-size, independent randomness to achieve perfect secrecy of the shares.

The CSPRNG variant must not be advertised as mathematically resistant to P = NP.

### Out of scope

The first spec does not cover:

- encryption of the whole system disk;
- protection against malware on an unlocked Mac;
- protection against an attacker holding both shares at the same time;
- synchronization via iCloud;
- sharing folders over the Internet;
- dedicated xordock operators;
- three shares and 2-of-3 schemes;
- a finished product with certified resistance to P = NP.

### Success criteria

- **SC-01.** The user creates a protected folder, mounts it, writes and reads files, and their contents after remounting are identical.
- **SC-02.** Removing the SD card prevents new reads of shares that require its presence.
- **SC-03.** After restarting the Mac the folder remains inaccessible until the correct card is connected and authorization is performed.
- **SC-04.** Each block modification gets a new mask, without reusing the previous one.
- **SC-05.** A failure during a write ends with recovery of the last committed version or an unambiguous error, never silent corruption.
- **SC-06.** Functional tests, failure tests and a security review for the prototype scope are completed.

## 4. Artefact Inventory

| Artefact | State | Responsibility |
|---|---|---|
| xordock repository | exists | Code and change history |
| `README.md` | exists | Current project state |
| `specs/S001-xordock-secure-folders.md` | exists | This spec |
| `CHANGELOG.md` | exists | Actually delivered features |
| `MANIFEST.md` | exists (to be extended with S001 decisions) | Implementation and security rules |
| `docs/threat-model.md` | to be created | Adversary model and protection boundaries |
| macOS application | to be created | Mounting and handling the folder |
| Virtual file system layer | to be created | File operations and reconstruction |
| Data splitting engine | to be created | Creating and joining shares |
| Randomness layer | to be created | Providing masks and validating the source |
| Share A store | to be created | Data on the internal SSD |
| Share B store | to be created | Data on the SD card |
| Transaction consistency mechanism | to be created | Safe committing of changes |
| Diagnostic and recovery tool | to be created | Consistency checking without revealing content |
| Automated tests | to be created | Evidence of operation and regressions |
| User documentation | to be created | Safe use of the card |
| FSKit / macFUSE evaluation | [TO BE DECIDED] | Choice of supported macOS integration |
| Spec of the true randomness source | [TO BE DECIDED] | Separate condition for the information-theoretic variant |

Repository state as of 2026-11-22: CDD artefacts only, no code.

## 5. Functional Domain Changes

### 5.1. Target state

The user sees one protected folder. Physically, files have two shares stored on different media. Applications receive plaintext data only during authorized use of the mounted folder.

### 5.2. Creating the folder

- **FR-01.** The user specifies the location of share A on the Mac.
- **FR-02.** The user specifies the SD card intended for share B.
- **FR-03.** The system checks the identity of the medium, writability, available space and compatibility with the required format.
- **FR-04.** The application initializes a new protected folder and records the binding of its shares.
- **FR-05.** The user sets up additional access authorization, e.g. a password, provided it does not weaken the secret-splitting mechanism.
- **FR-06.** The folder must not be created partially without the possibility of unambiguously cancelling or completing the initialization.

### 5.3. Writing data

- **FR-07.** Writing each new block requires a fresh mask.
- **FR-08.** Each mask may be used to create shares of only one version of a given block.
- **FR-09.** Shares A and B must be written in matching versions.
- **FR-10.** A write is considered durable only after the required persistence and commit operations of both shares have completed.
- **FR-11.** Lack of space or lack of access to the SD card must stop the write before success is announced.

### 5.4. Reading data

- **FR-12.** The system reads the corresponding fragments from both shares.
- **FR-13.** The original block is reconstructed in main memory by the XOR operation.
- **FR-14.** A mismatch of identifiers, versions or share structure results in an error, not in returning arbitrary data.
- **FR-15.** The program must not deliberately persist plaintext data in a technical folder or diagnostic log.

### 5.5. SD card handling

- **FR-16.** The system recognizes the correct medium and the correct set of shares, not relying solely on the volume name.
- **FR-17.** Connecting a different card must not cause shares from different folders to be combined.
- **FR-18.** Losing the connection with the card during a write must put the folder into an error state and prevent further commits of changes.
- **FR-19.** Safe unmounting requires completing active operations and persisting the data.
- **FR-20.** After unmounting, new reads and writes are not available. Data previously read into application memory must not be considered automatically removed.

### 5.6. File changes and history

- **FR-21.** Overwriting a file must not reuse the mask from the previous version.
- **FR-22.** Resize, delete, move and rename operations must preserve share consistency.
- **FR-23.** The system must explicitly define its behavior with respect to versioning, backups, logs and snapshots.
- **FR-24.** Creating a hidden full copy of the plaintext folder on any medium is not allowed.

### 5.7. Security model

**Adversary covered by the target guarantee:**

- obtains a full copy of share A;
- may have unlimited computing power, including algorithms resulting from a practical P = NP;
- knows the full code and data format;
- may analyze earlier copies of share A;
- does not obtain share B or plaintext copies of the data;
- does not control the computer while the mounted folder is in use.

**The mathematical guarantee concerns the confidentiality of the content**, assuming independent, uniformly random masks of adequate length and correct implementation of the protocol.

In particular, a single share should not reveal the content of the data. This does not automatically mean hiding sizes, activity times, information from the operating system, or resistance to modification of shares by an attacker.

**The CSPRNG variant** does not meet the condition of unconditional confidentiality. It is to be unambiguously labelled as a practical prototype, with no claim of resistance to P = NP.

### 5.8. Security of data outside the folder

- **FR-25.** The application is to document the limitations related to temporary files, swap memory, Finder previews, recent documents history and the behavior of office applications.
- **FR-26.** The system must not promise that all macOS programs remove their plaintext copies after a document is closed.
- **FR-27.** Independent use of FileVault is recommended as additional protection of the rest of the computer's data.

## 6. Critical Path

The goal is one working folder whose contents can be read only when two matching shares are available.

1. **CP-01 — Data model.** Define the format and versioning rules of the shares. Confirm with tests that fresh masks create shares correctly and that they are not reused.
2. **CP-02 — File operations.** Implement basic creating, reading, writing and deleting of files in two test locations.
3. **CP-03 — macOS integration.** Choose and verify a supported virtual file system mechanism. Make the folder available in Finder.
4. **CP-04 — SD card.** Replace one test location with a real SD card. Verify its connecting, identification, unmounting and remounting.
5. **CP-05 — Write consistency.** Implement recovery after an incomplete operation and check the behavior during simulated failures.
6. **CP-06 — Security tests.** Check share isolation, no mask reuse, the extent of plaintext copies and resistance to invalid input data.
7. **CP-07 — Demonstration prototype.** The user writes a file to the folder in Finder, unmounts it, disconnects the card and cannot reopen the folder. After connecting the correct card they can read the file unchanged.

**Completion condition:** all MVP gates met, with no unresolved bugs causing data loss in the defined test model.

## 7. Control Gates

All gates are mandatory for the declared scope. An unmet gate blocks marking the corresponding release as ready.

| ID | Pass condition | Verification |
|---|---|---|
| CG-01 | Reading after writing gives identical bytes | Unit and E2E tests for files of various sizes |
| CG-02 | A copy of share A alone contains no plaintext content of the test files | Medium inspection and scenario tests |
| CG-03 | A copy of share B alone contains no plaintext content of the test files | Medium inspection and scenario tests |
| CG-04 | Each modification gets a new mask | Instrumented tests with unique mask identifiers |
| CG-05 | Absence of the card blocks access to the folder | E2E test |
| CG-06 | A wrong card is rejected | E2E test |
| CG-07 | A write failure does not cause silent corruption | Failure tests with interruption at every commit stage |
| CG-08 | Parallel operations do not cause shares to be mixed up | Concurrency tests |
| CG-09 | Logs contain no file contents or masks | Automated analysis of test logs |
| CG-10 | The CSPRNG is not presented as a guarantee of resistance to P = NP | Verification of documentation and interface |
| CG-11 | Security analysis of the current and previous specs detects no unresolved critical regressions | Security review after each round |

### Additional gates for the future P = NP variant

- **IT-01 — Entropy validation.** The documented mask source must provide a sufficient amount of independent entropy for the whole protected block. Passing statistical randomness tests alone is not proof.
- **IT-02 — Proof of confidentiality.** An independent cryptographic analysis must confirm that one share does not reveal content in the defined adversary model, also taking into account updates and write history.
- **IT-03 — Verification of system boundaries.** Channels of content disclosure outside the share stores must be investigated, in particular swap memory, application caches, indexing and backups.
- **IT-04 — Honest claims.** Until IT-01–IT-03 are met, no version of the program may be described as providing full resistance to P = NP.

## 8. Expected Outcome

### Direct effect

A macOS application is created that allows creating and using a folder whose data physically requires two media. The operating system still works without the SD card. After it is disconnected, the protected folder becomes unavailable for new operations.

### Measurable effect

The following must be measured in the prototype:

- read and write throughput;
- latency of operations on small files;
- mount and unmount time;
- capacity overhead on both media;
- effectiveness of detecting interrupted writes.

The results are to be recorded in the test documentation. Performance thresholds for the production release will be set based on prototype measurements, not guessed values.

**Prototype measurements (2026-11-22, MacBook Pro Apple Silicon, macOS 27.0, A: internal SSD with FileVault, B: HFS+ SD card in the built-in reader):**

| Measurement | Result | How to repeat |
|---|---|---|
| Write 20 MiB, vault layer | 1.9 s (10.4 MiB/s) | `XORDOCK_PERF_A=… XORDOCK_PERF_B=/Volumes/… go test -run TestPerf -v ./internal/vault` |
| Read 20 MiB, vault layer | 33 ms (~600 MiB/s) | as above |
| Small file, vault layer (1 commit) | ~120 ms | as above |
| `cp` 20 MB into the mounted folder | 1.85 s | E2E, §9.9 |
| Small file through the mount (2 commits) | ~215 ms | E2E, §9.9 |
| Mount / unmount | 1.0 s / 0.02 s | E2E, §9.9 |
| Capacity overhead | 2 × data + 57 B per object (block ≤ 1 MiB) + index | format in `internal/vault/format.go` |
| Detecting interrupted writes | 100% of test scenarios (5 stages × successive occurrences) | `TestCrashAtEveryStage` |

The first version with a 64 KiB block gave 1.4 MiB/s (4 × fsync per block). A 1 MiB block and persisting folders once per commit gave a ~7× speedup without changing the guarantees. The bottleneck is the number of fsync calls on the SD card; small files are limited by commit time (2 × F_FULLFSYNC of superblocks).

### Side effects

- Loss of one share may mean irreversible data loss.
- SD cards may be slower and less durable than internal SSDs.
- During work, plaintext data is available to applications in RAM.
- Performance depends on the slower medium and the number of synchronization operations.
- An ordinary SD card can be copied; it is not automatically a secure hardware token.

### Rollback

Rolling back the program must not require leaving a plaintext copy of the data on the internal SSD. Before rollback the user must be able to deliberately export the files to a specified location with both shares connected. The export must clearly state that the target files will no longer be protected by the secret-splitting mechanism.

## 9. Additional Information

### 9.1. Rejected alternatives

- **Whole system disk:** rejected for the MVP due to the complexity of booting macOS, drivers and recovery after failure.
- **Card with only an AES key:** does not achieve the goal of information-theoretic protection of shares under P = NP.
- **Traditional RAID 1:** stores full copies of data, so losing a single medium reveals everything.
- **OTP for network communication:** outside the scope of this spec.
- **Porting the existing SafeFS without an audit:** rejected, because it requires a separate check of the randomness generator, current compatibility with macOS and write safety.

### 9.2. Technical debt and risks

- **R-01.** Choosing a CSPRNG for the prototype means no strict P = NP guarantee.
- **R-02.** Ordinary applications may leave plaintext copies of documents outside the virtual folder.
- **R-03.** The SD card may be unexpectedly disconnected.
- **R-04.** XOR splitting alone does not guarantee integrity or authenticity of the data.
- **R-05.** A backup of one share is not enough to recover the data.
- **R-06.** Leakage of sizes, access times and certain metadata is not automatically eliminated.

### 9.3. External dependencies

- A virtual file system mechanism supported by the selected macOS releases.
- SD media providing the required durable write operations.
- Available tools for testing I/O errors and power failures.
- An independent review of the cryptographic design.
- A separate evaluation of the entropy source for the information-theoretic variant.

### 9.4. Open questions — require decisions

- **Q-01.** Should the first version support only the latest macOS release, or also older supported versions?
- **Q-02.** Should the protected folder be accessible via FSKit, macFUSE or another supported interface? The decision should be made after comparing stability and permissions.
- **Q-03.** Should the prototype use the CSPRNG only for demonstration, with the information-theoretic mode created as a separate spec?
- **Q-04.** How should consistent backups of both shares be handled, so that an accidental SD failure does not mean permanent data loss?
- **Q-05.** What integrity requirements apply against an attacker who can modify one share? This is a separate problem from confidentiality and requires an explicit cryptographic decision.
- **Q-06.** Should the first version support editing arbitrary documents in applications that create automatic copies and temporary files, or only an agreed list of programs?

### 9.5. Process

The project is run according to Rondo / CDD (see `CLAUDE.md`): each change has its own spec in `specs/`, `README.md` describes the actual state, `CHANGELOG.md` documents delivered changes, requirements lead to tests and gates, and after each round a security analysis of the current and earlier functionality is performed. A detected gap requires documentation, a test reproducing the problem and a fix before the round is closed.

### 9.6. Source materials

- Rondo / Context-Driven Development: https://workwithrondo.com/
- SafeFS: https://github.com/safecloud-project/SafeFS
- Lecithine: https://sourceforge.net/projects/leci/
- Apple documentation on FSKit and macOS security mechanisms.
- Literature on the one-time pad, perfect secrecy and secret sharing.

### 9.7. Name history

Working name in the first draft: "OpenTrust Secure Folders" / "OpenTrust Storage". Owner's decision (2026-11-22): the project and the product are named **xordock**.

### 9.8. Decisions (2026-11-22)

The owner instructed to accept the recommendations without further questions.

- **Q-01:** only macOS 27 (Apple Silicon), i.e. the version we work on.
- **Q-02:** the prototype mounts the folder via a local WebDAV server (127.0.0.1) and the macOS built-in `mount_webdav`. Reasons: no Xcode on the workstation (FSKit requires an app extension signed via Apple Developer), macFUSE requires a kernel extension and lowered security on Apple Silicon. WebDAV works without administrator privileges. **Cost:** the macOS WebDAV agent caches file contents on the SSD (finding F-01). The target integration is S002.
- **Q-03:** the prototype uses a CSPRNG (`crypto/rand`). The information-theoretic mode is a separate spec (IT-01–IT-04 unchanged).
- **Q-04:** no automatic backups in the prototype. Recommendation in `docs/usage.md`: copy the vault folder from the card to a second card stored separately, with the vault unmounted. The full solution in a separate spec.
- **Q-05:** each share has its own SHA-256 checksum and a header binding it to the vault, side and object. This detects accidental damage and mixing up of shares. It does **not** detect deliberate substitution (XOR is malleable, R-04). A hash of share A stored on B (and vice versa) was rejected, because it would let an unbounded adversary with one share verify hypotheses about the content (breaks G-06). Integrity against an adversary is a separate spec.
- **Q-06:** no list of applications; limitations described in `docs/usage.md` (FR-25, FR-26).
- **FR-05 (password):** deferred to a separate spec. In the prototype the authorization (SC-03) is the correct card and the logged-in macOS user session.
- **Implementation language:** Go, one dependency: `golang.org/x/net/webdav` (official Go module).
- **Lock:** the vault is opened by only one process at a time (`flock` on `lock` in share A).
- **FileVault:** `xordock mount` refuses to run without FileVault (the `--allow-no-filevault` flag deliberately bypasses this), because of F-01.

### 9.9. Implementation and evidence

| Gate | Evidence | Result |
|---|---|---|
| CG-01 | `TestRoundTripAfterReopen`, `TestWebDAVLifecycle`, `TestRandomWritesMatchModel`; E2E: `cp`/`cmp` 20 MB and 3 MB through the mount, after remounting | ✅ |
| CG-02 | `TestSharesHoldNoPlaintext`; E2E: `grep` for the marker and file name in share A on the SSD | ✅ |
| CG-03 | as above, share B on the card | ✅ |
| CG-04 | `TestEveryWriteGetsFreshMask` (instrumented mask source, 16 masks, 0 repeats), `TestFreshMaskEachSplit` | ✅ |
| CG-05 | `TestMissingCardBlocksOpen`; E2E: unmounting the card → folder unmounted in ≤ 1 s, `mount` without the card rejected | ✅ |
| CG-06 | `TestWrongCardRejected` | ✅ |
| CG-07 | `TestCrashAtEveryStage` (13 scenarios: every occurrence of every stage when writing 4 blocks and the index), `TestInterruptedInitFinished`, `TestTornTempFilesCleaned`, `TestCorruptShareIsError`, `TestCorruptSuperIsError` | ✅ in the test model; no test with a physical power cut |
| CG-08 | `TestConcurrentWrites`, `TestConcurrentPUT` (with `-race`), `TestSecondOpenLocked` | ✅ |
| CG-09 | `TestLogsHoldNoSecrets`, `TestStripPaths` | ✅ |
| CG-10 | `TestKindIsHonest`; message "Masks: CSPRNG (prototype)… NO guarantee of resistance to P = NP" at `init` and `mount` | ✅ |
| CG-11 | `docs/security/2026-11-22-round-S001.md` | ✅ no critical regressions; F-01 (high) open → S002 |

CP-07 (demonstration) was performed via `mount_webdav`, the same path Finder uses: write, unmount, no card → no access, card back → file unchanged. Finder was not clicked through (work done without the owner present).

---

**Spec closing condition:** approved MVP scope, settled macOS integration, accepted prototype security model and accepted gates CG-01–CG-11.

**P = NP security status:** a research goal, unconfirmed for the first release. Requires a separate validation process IT-01–IT-04.
