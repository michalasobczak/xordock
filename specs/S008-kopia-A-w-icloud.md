# S008 — Share A copy in iCloud Drive

- **Status:** delivered (G1–G6 tests, E2E on USB sticks and iCloud); G7 (app) awaits the owner's test
- **Date:** 2026-11-22
- **Related:** S004 (§5.5–5.6, B-01–B-05, R-02), S006, S007

## 1. TL;DR

From now on a backup writes one recovery point in two places:

- the snapshot's B shares to the spare card;
- the A shares of the same snapshot to `iCloud Drive/xordock/<vault>`.

After losing the SSD, data is recovered from the A copy in iCloud together with the spare card or with the working card. This is confirmed by a test that deletes the A directory and recovers the bytes from the copy.

## 2. Abstract

After S004, share B has two copies (working card and spare card), while share A has only one, on the SSD. Losing the SSD (failure, theft of the Mac, reinstall) therefore means losing the data. The spare card does not help here, because recovery requires A.

The owner noticed the asymmetry: two keys on two media also require two copies of A. It was verified that iCloud on macOS does not copy `~/Library/Application Support`. Time Machine does, but its disk is sometimes disconnected.

A shares can safely be kept outside the Mac on the condition that the same place never receives any B. The protection is then practical: the iCloud account plus any B card make a complete set.

Owner's decisions (2026-11-22):

- **When:** together with the backup. One point, an A + B pair from the same snapshot.
- **Retention:** as on the spare card. Pruning points covers both places.
- **Location:** iCloud Drive, hard-coded: `~/Library/Mobile Documents/com~apple~CloudDocs/xordock/<vault>`.
- **Advanced Data Protection:** a one-time warning in the app with confirmation. The CLI prints a warning on every backup with an A copy.

## 3. Objective

- **Goals:**
  - K-01 `backup` copies the snapshot's A shares to iCloud Drive under the same point identifier as the B copy. The A copy gets its own header (`side:A`, `role:backup`, `media`) and a `points/<id>` READY record after verification.
  - K-02 When iCloud Drive is missing (the directory does not exist), the B backup works as before, and the CLI and the app clearly say that there is no A copy. `--no-icloud` deliberately skips it.
  - K-03 `backup prune` also prunes the A copy (the newest point stays, after a completeness check).
  - K-04 Recovery without the SSD: `recover --from-icloud --spare <spare or working card>` (list, verify, export, restore). The points come from the records of the copies that have the backup role.
  - K-05 The app:
    - shows the state of the A copy (last point);
    - warns once before the first copy to iCloud (ADP, 2FA);
    - shows copies in iCloud for which this Mac has no share A, with a "Recovery…" button.
- **Out of scope:**
  - detecting whether ADP is enabled (no public API);
  - other clouds;
  - an A copy without a B backup;
  - encrypting the A copy with an additional password.
- **Success criteria:** the gates from §7.

## 4. Artefact Inventory

| Artefact | Type | State |
| --- | --- | --- |
| `internal/vault/backup.go` | code: `BackupPair`, generalized side copying and pruning | to change |
| `internal/vault/recovery.go` | code: recovery from the A copy | to change |
| `cmd/xordock/backup.go`, `app.go`, `main.go` | CLI code | to change |
| `macos/Sources/*` | app | to change |
| `docs/recovery.md`, `docs/format-v1.md` | documentation | to change |

## 5. Functional Domain Changes

- **A copy layout:** the same as the spare card (`xordock.json`, `obj/xx/<oid>`, `points/<id>`), but the header has `side:"A"`. Shares are copied byte for byte, so the masks do not change.
- **Order:**
  1. Check the space on both targets.
  2. Pin the snapshot for both media (two `PointRef`s with the same `Point`).
  3. B copy, verification and READY point on the card.
  4. A copy, verification and READY point in iCloud.
  5. Ready flags in the index.

  An interruption before step 4 leaves the previous A points untouched. An A copy failure after the card's READY does not invalidate the B point; the message says that the A copy failed.
- **Security rules:**
  - The A copy target must not be on the device of the working or spare card, nor inside the share A or B directory.
  - No B share ends up in the A copy directory: this is checked by `decodeShare` of side A before writing.
- **Recovery:** `OpenRecovery(a, b)` accepts as A a live directory or a copy, and as B a spare or working card. At least one side must be a copy with point records. The copy is neither locked nor modified.
- **iCloud:**
  - Copy files may be evicted from disk (storage optimization). Reading during recovery downloads them.
  - An atomic write (tmp + rename) does not leave halves of files in iCloud as finished objects, because the target name appears only after the rename.

## 6. Critical Path

1. Engine: `BackupPair`, pruning the A copy, recovery from the copy, with tests.
2. CLI: default copy to iCloud on `backup`, `--no-icloud`, `recover --from-icloud`, the `aCopy` field and the `orphanCopies` list in `vaults`.
3. App.
4. E2E: backup to a USB stick + iCloud, then recovery from the copy with the A directory renamed.

## 7. Control Gates

| # | Gate | Condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | SSD loss | after deleting the A directory, the point's data is recovered from the A copy + spare card, byte for byte | `TestRecoverAfterLosingSSD` | commit |
| G2 | Loss of SSD and spare card | A copy + working card restores the point | same test | commit |
| G3 | **Negative:** no B in the A copy | every file in the A copy is a side A share or a header/record; no plaintext content | `TestACopyHoldsNoBNoPlaintext` | commit |
| G4 | **Negative:** A copy target ≠ card | refusal when the target is in the B directory or on the spare card | test | commit |
| G5 | Interrupted A copy | the previous A point is still recoverable, the B point is READY | `TestInterruptedACopy` | commit |
| G6 | Retention | `prune` leaves only the newest point on both targets | test | commit |
| G7 | App | ADP warning once, A copy state, recovery without A | E2E | release |

## 8. Expected Outcome

- Each of the three media (SSD, working card, spare card) can be lost individually without losing the data of the last point.
- Cost: space in iCloud ≈ size of the snapshot data.
- Rollback: `--no-icloud`; deleting the `iCloud Drive/xordock` directory does not touch the vault.

## 9. Additional Information

- **Risk:** the iCloud account becomes a carrier of share A. Whoever takes over the account needs only one card. This is mitigated by 2FA, ADP and keeping the cards apart.
- **Rejected:**
  - moving the live A to iCloud Drive: files evicted from disk, sync conflicts, no offline work;
  - mounting the plaintext volume in Documents: plaintext content in the cloud.

## Implementation (2026-11-22)

- **Engine:**
  - `Backup` and `BackupPair` use a single path `backup` → `copySide` for side A or B.
  - Test hooks: `backup-*` for B, `acopy-*` for A.
  - `prepareCopy` rejects targets located in share directories and on the working card's device. `BackupPair` rejects an A copy on the spare card's device. Full physical medium checks belong to S006.
  - `PruneACopy`.
  - `OpenRecovery` accepts the A copy and the working card. When both sides are copies, the points are their intersection. Copies are not locked.
- **CLI:**
  - `backup` makes an A copy by default when iCloud Drive exists (`--no-icloud`); `backup prune` prunes both targets;
  - `recover`/`points --from-icloud`;
  - `status` shows the A copy;
  - `vaults`: fields `icloud`, `aCopy` and the `orphanCopies` list (copies without share A on the Mac).
- **App:**
  - ADP warning once (`UserDefaults adpWarningAccepted`);
  - a "Share A copy (iCloud)" row and a warning when it is missing;
  - an "A copies in iCloud without the SSD" section with recovery;
  - a "Share A from the iCloud copy" toggle in recovery.
- **Tests:**
  - `TestRecoverAfterLosingSSD` (G1, G2), `TestACopyHoldsNoBNoPlaintext` (G3), `TestACopyRejectsCardLocations` (G4), `TestInterruptedACopy` (G5, 3 stages), `TestPruneBothCopies` (G6), `TestACopyInState`;
  - Swift: `aCopyStateAndArgs`.
- **E2E (2026-11-22):**
  - temporary vault: A in a temporary directory, B on the `xor_primary` USB stick, spare on `xor_backup`, A copy in the real iCloud Drive;
  - a 3 MB file with a marker dropped in through Finder, snapshot and pair backup;
  - A directory renamed ("SSD loss"), then `recover --from-icloud verify/export` with the spare card and with the working card: `cmp` identical;
  - no marker in the A copy, on the cards or in the old A;
  - `vaults` showed the copy as orphaned;
  - everything removed from the cards and from iCloud Drive.
