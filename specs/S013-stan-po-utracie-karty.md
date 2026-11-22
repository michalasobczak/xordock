# S013 — Clear state after the working card is removed

- **Status:** delivered
- **Date:** 2026-11-22
- **Related:** S004 (SC-03, F-04), S009, S011

## 1. TL;DR

When the working card is pulled out of an open vault, the app shows one plain sentence instead of the raw log (the log stays under "Details"), drops the stale "opened in Finder" banner, and the tiles stop claiming "no backup" when the backup simply cannot be read without the card.

## 2. Abstract

The owner pulled the card out of an open vault during the tutorial run (2026-11-22). The emergency unmount worked as designed (S004 SC-03), but the app:

- showed the last lines of the mount log verbatim: timestamps, vault ID, four overlapping messages;
- kept the green banner "Vault "sejf" opened in Finder." next to the error;
- showed "Spare card: no backup" and an orange iCloud tile, although a backup from 23:18 existed. Without the working card the CLI cannot read the index (`Peek` needs both shares), so `lastBackup` is unknown, not absent.

The owner asked for these three fixes before the screenshot goes into the README tutorial.

## 3. Objective

- **Goals:**
  - C-01 Emergency unmount caused by card removal → error banner: "The working card was removed while vault "X" was open. The vault was locked and unmounted; unsaved changes were lost. Put the card back and open the vault again." Other mount failures → "Vault "X" was unmounted because of an error." The log lines are available under "Details".
  - C-02 When the mount process ends, the success banner from opening the vault is cleared.
  - C-03 Without the working card: the spare card tile shows "unknown" with the note "connect the working card" (neutral, not a warning); the iCloud tile shows the date of the last share A copy from iCloud (readable without the card) without a backup-age warning.
- **Out of scope:** reading backup state from the spare card without the working card; CLI message changes.
- **Success criteria:** gates in §7.

## 4. Artefact Inventory

| Artefact | Kind | State |
| --- | --- | --- |
| `macos/Sources/XordockCore/Health.swift` | code: tiles without card, mount failure summary | to change |
| `macos/Sources/XordockApp/AppModel.swift`, `MainView.swift` | code: error details, stale banner | to change |
| `macos/Tests/XordockCoreTests/CoreTests.swift` | tests | to change |
| `docs/ui/3-no-card-light.png` | render | to refresh |

## 5. Functional Domain Changes

- Classification uses the CLI log text ("working card removed"/"working card unavailable"); unknown text falls back to the generic sentence, never to silence.
- No behavior change in the engine or CLI.

## 6. Critical Path

Core rules with tests → app wiring → render → build.

## 7. Control Gates

| # | Gate | Condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | Summary | card-removal log → card sentence; other log → generic sentence; both ≤ 200 chars and contain the vault name | `swift test` | commit |
| G2 | Tiles without card | spare: "unknown", neutral; iCloud: date from the A copy, not a warning | `swift test` | commit |
| G3 | No stale banner | after the mount process exits, no success banner remains | review | commit |
| G4 | No regressions | `swift test`, `go test -race ./...` green | test | commit |

## 8. Expected Outcome

After a card removal the user reads one sentence and sees an honest dashboard. Rollback: revert.

## 9. Additional Information

The README tutorial screenshot of this state (`docs/tutorial/13-card-pulled-out.webp`) predates the fix; it is replaced when the owner takes a new one.

## Implementation (2026-11-22)

- `MountFailure.summary` / `isCardRemoval` in `XordockCore/Health.swift`; the app keeps the log in `errorDetails`, shown under "Details" in the error banner.
- `CLI.logTail` now reads only the latest mount session (after the last `--- <time> mount` marker), so details never mix in older sessions.
- On mount-process exit the success banner and its output are cleared.
- Tiles: without the working card the spare tile is neutral "unknown" ("connect the working card"); the iCloud tile shows the A copy date without the backup-age warning.
- Tests: `tilesStates` (updated), `iCloudTileWithoutCard`, `mountFailureSummary`, `logTailLatestSession`; Swift 22/22. Render `docs/ui/3-no-card-light.png` refreshed.
- Security review: text and presentation only; the emergency unmount itself (S004 SC-03) is unchanged. The raw log is still available (Details) and contains no file names (CG-12). No findings.
