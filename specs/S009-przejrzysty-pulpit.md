# S009 — Clear vault dashboard

- **Status:** delivered (G1–G5); appearance acceptance by the owner in the running app
- **Date:** 2026-11-22
- **Related:** S007, S008

## 1. TL;DR

The vault view in the app changes from a text-heavy form into a dashboard:

- one large status with one main button ("what to do now");
- four media tiles (SSD/A, working card/B, spare card/B copy, iCloud/A copy) with ✓/⚠︎ and a date;
- technical details collapsed.

The result is confirmed by tests of the status rules and by renders of the view to PNG.

## 2. Abstract

After the first tests of the app (S007, S008), the owner judged the window as unclear:

- too much text;
- it is not clear what to do next;
- the state of the media is unclear.

He chose the "dashboard with tiles" layout (2026-11-22).

## 3. Objective

- **Goals:**
  - P-01 **Status with a single recommendation.** The first matching rule (priority order):
    1. vault error;
    2. no working card;
    3. no backup;
    4. backup older than 7 days;
    5. no A copy in iCloud;
    6. vault open;
    7. everything fine.

    Each state has a title of up to 5 words, one sentence of explanation and at most one main button.
  - P-02 **Four tiles:** medium name, role in one word, state ✓/⚠︎/–, date of the last copy or volume name.
  - P-03 **Secondary actions** in one row of small buttons. "More" holds fsck, log and details.
  - P-04 **Technical details** (paths, masks, commit, snapshots) in a collapsible section, collapsed by default.
  - P-05 In the vault list, a dot in the state's color instead of a description.
  - P-06 Security texts (CG-14, ADP, plaintext export) stay, but next to the action they concern.
- **Out of scope:** changes to the CLI and the format, the backup and recovery sheets (apart from shortening texts).
- **Success criteria:** the gates from §7 and the owner's acceptance.

## 4. Artefact Inventory

| Artefact | Type | State |
| --- | --- | --- |
| `macos/Sources/XordockCore/Health.swift` | code: status and tile rules | to create |
| `macos/Sources/XordockUI/Dashboard.swift` | code: dashboard view (library, renderable) | to create |
| `macos/Sources/XordockPreview/main.swift` | tool: renders the dashboard to PNG for sample states | to create |
| `macos/Sources/XordockApp/*` | code | to change |

## 5. Functional Domain Changes

- The dashboard state is computed by a pure function `VaultHealth.evaluate(vault, icloud:)`. The view contains no decision logic.
- The spare card tile shows the date of the last READY point and whether the card is currently connected.
- iCloud tile: "off" when iCloud Drive is unavailable.

## 6. Critical Path

1. `Health.swift` with tests.
2. `XordockUI` with the dashboard.
3. Wiring into the app.
4. PNG renders and review.
5. Building the `.app`.

## 7. Control Gates

| # | Gate | Condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | Rules | each of the 7 states and the priorities | `swift test` | commit |
| G2 | Tiles | states ✓/⚠︎/– for connected and disconnected cards, iCloud off | `swift test` | commit |
| G3 | Brevity | title ≤ 5 words, explanation ≤ 1 sentence (≤ 120 characters) for every state | `swift test` | commit |
| G4 | Appearance | PNG renders of 4 states reviewed: light and dark mode, no truncated texts | review | commit |
| G5 | No regressions | S007/S008 tests green, S007 G4/G5 (no shell, no access to shares) | `swift test` | commit |

## 8. Expected Outcome

On opening the window it is immediately clear whether things are fine and what to do. Rollback: restoring the previous `VaultDetail`.

## 9. Additional Information

No access to the owner's screen (macOS permissions), so the appearance is checked with `ImageRenderer` renders.

## Implementation (2026-11-22)

- `XordockCore/Health.swift`: `VaultHealth.evaluate` (7 states, P-01 priorities), `VaultInfo.tiles(icloud:)` (P-02), dates in Polish regardless of the system language.
- `XordockUI` (new library):
  - `Dashboard`: status with the main button, 4 tiles, action row, collapsed details;
  - `HealthDot`;
  - custom button styles (pure SwiftUI, renderable);
  - the action that is the main button is not repeated in the row.
- App:
  - the dashboard replaces the `VaultDetail` form; state dots in the list; the menu bar menu shows the status title;
  - messages after actions are one sentence, and the full CLI output is under "Details";
  - shorter banners.
- `XordockPreview`: `swift run XordockPreview <directory>` renders 4 states × light/dark mode to PNG. Examples in `docs/ui/`. This is a developer tool; it does not go into the `.app`.
- **Gates:**
  - G1–G3: tests `healthPriorities`, `tilesStates`, `healthTextsAreShort`;
  - G4: renders reviewed (fixed a duplicated "Open vault" button, English dates, alignment of the "This Mac" tile);
  - G5: 13 Swift tests green, including the ban on the shell and on access to shares, which covers the new sources.
