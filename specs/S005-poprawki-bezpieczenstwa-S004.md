# S005 — Fixes from the S004 round security analysis

- **Status:** delivered
- **Date:** 2026-11-22
- **Related:** S004, `docs/security/2026-11-22-round-S004.md` (R4-01, R4-02)

## 1. TL;DR

Two gaps found in the analysis after S004 were fixed: `backup prune` does not delete older points until the newest one is complete, and an index with paths outside the tree is rejected. Proof: tests `TestPruneRefusesIncompleteNewest` and `TestIndexPathsValidated`.

## 2. Abstract

After implementing S004 the code was reviewed for regressions and gaps. `PruneBackup` only checked whether the index of the newest point could be assembled, and then deleted older points and their shares. If any block of the newest point were damaged, the user would lose the only good copy, contrary to B-04 and the rule "the last backup must not be deleted automatically".

The index loaded from the shares was not validated. A damaged or deliberately substituted share (XOR malleability, F-03) could contain a path with `..`, and `export` and `recover export` would write a file outside the target folder. Integrity against an adversary is out of scope for the MVP, but writing outside the specified folder is harm to the host, and the fix is cheap.

## 3. Objective

- **Goals:** prune deletes anything only after checking both shares of every object of the newest point; an index with a non-canonical path or an entry without a parent folder is `ErrCorrupt`; export refuses a path leading outside the folder.
- **Out of scope:** share authentication (F-03, a separate cryptographic decision).
- **Success criteria:** both tests green, full `go test -race ./...` green.

## 4. Artefact Inventory

| Artefact | Kind | State |
| --- | --- | --- |
| `internal/vault/backup.go` (`PruneBackup`) | code | changed |
| `internal/vault/vault.go` (`Index.validate`, `loadIndexFrom`) | code | changed |
| `internal/vault/recovery.go` (`ExportTo`) | code | changed |
| `internal/vault/backup_test.go` | test | changed |

## 5. Functional Domain Changes

- `PruneBackup`: before deleting anything it decodes share B (spare card) and A (SSD) of every object of the newest point; error → nothing is deleted.
- `loadIndexFrom` calls `Index.validate`: every path equals `Clean(p)`, starts with `/`, and has a parent that is a folder.
- `ExportTo` additionally checks that the target path lies within the export folder (defense in depth).
- Impact on data: none; correct indexes always met these conditions (operations use `Clean`).

## 6. Critical Path

Reproducing tests (red) → fixes → tests green → CHANGELOG entry.

## 7. Control Gates

| # | Gate | Pass condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | Prune does not lose the only good copy | with a damaged block of the newest point, prune returns an error, both points remain | `TestPruneRefusesIncompleteNewest` | commit |
| G2 | No paths outside the tree | an index with `..`, a relative path, `//`, a trailing `/` or without a parent is rejected | `TestIndexPathsValidated` | commit |

## 8. Expected Outcome

Retention is safe even with a damaged medium; export does not go outside the target folder. Rollback: revert the commit (no format changes).

## 9. Additional Information

Found in the S004 round security analysis (CLAUDE.md requirement: analysis after every round, findings fixed like a regular change).
