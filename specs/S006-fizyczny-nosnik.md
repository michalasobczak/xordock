# S006 — Shares A and B on different physical devices

- **Status:** draft (owner's decisions collected 2026-11-22; implementation after S007)
- **Date:** 2026-11-22
- **Related:** S004 (D-01, B-03), S005, S007

## 1. TL;DR

xordock must refuse to work when share A, the working card B or the spare card lie on the same **physical** disk. Today only the volume number (`st_dev`) is compared. The result will be confirmed by tests with substituted disk detection and a test with a disk image located on the SSD.

## 2. Abstract

`vault.Create` and `prepareSpare` compare the `st_dev` of the paths. Each volume has its own number, so the following pass the check:

- a `.dmg` image or sparsebundle lying on the SSD next to A. Confirmed 2026-11-22: `init` accepted B on an image in `scratchpad`;
- a second partition or second APFS volume on the same disk;
- a second volume on the same USB stick, used as the spare card.

In addition, the check result is ignored (`same, _ := sameDevice(...)`). When `MkdirAll` in `/Volumes` fails, the check lets it through (fail-open), and `Create` manages to write the A header before writing B fails. This happened to the owner with a typo in the volume name: an orphaned `xordock.json` was left in A. The same typo run with `sudo` created a `/Volumes/<typo>` folder on the system disk. Running as root also leaves A files owned by root.

The check runs only at `init` and backup. B moved to the SSD later (a copied folder) opens without objection.

## 3. Objective

- **Goals (owner's decisions 2026-11-22):**
  - **When:** always. At `init`, at every vault open (mount, status, fsck, snapshot, export, backup), at backup (spare ≠ disk A and ≠ disk B) and at `recover` (spare ≠ disk A, new card ≠ disk A).
  - **Disk images:** we determine the physical disk on which the image file lies and compare it like an ordinary device.
  - **Undetermined device** (network volume, iCloud, `diskutil` error): refusal (fail-closed).
  - **Override:** none. Only unit tests in `_test.go` code disable the check.
  - No side effects on refusal: no folders in `/Volumes` on the system disk, no A header.
  - Proposal to be confirmed: refuse to work as root (`euid == 0`).
- **Out of scope:** detecting that two different physical disks are in one enclosure or RAID.
- **Success criteria:** the gates from §7 pass.

## 4. Artefact Inventory

| Artefact | Kind | State |
| --- | --- | --- |
| `internal/vault/media.go` | code: physical disk of a path (statfs → diskutil → APFS physical store → hdiutil image-path) | to be created |
| `internal/vault/vault.go`, `backup.go`, `recovery.go` | code | to be changed |
| `cmd/xordock/main.go` | code: `--card` must be a mount point | to be changed |

## 5. Functional Domain Changes

[TO BE DECIDED during implementation. Principle: physical disk identifier = whole disk after expanding the APFS physical store and image files, with limited recursion depth.]

## 6. Critical Path

[TO BE DECIDED]

## 7. Control Gates

| # | Gate | Condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | B on an image on the SSD | refusal | darwin test with `hdiutil` | commit |
| G2 | Detection error | refusal, nothing written | test with a substituted resolver | commit |
| G3 | Open checks A ≠ B | refusal | test | commit |
| G4 | Typo in `--card` | refusal without creating a folder in `/Volumes` | CLI test | commit |

## 8. Expected Outcome

It is impossible, even by mistake, to keep A and B on one physical device.

## 9. Additional Information

Order: the owner decided that the app (S007) comes first.
