# S010 — Tutorial: introduction and guidance through actions

- **Status:** delivered (G1–G3, G5); G4 and a walkthrough of the tutorial to be checked by the owner
- **Date:** 2026-11-22
- **Related:** S007, S008, S009

## 1. TL;DR

On every launch the app asks whether to start the tutorial. The "Don't ask at launch" option can later be reversed in Settings.

The tutorial has two parts:

- 6 screens with a "Next" button that explain the problem and the solution;
- guidance on a real vault: a bubble points at a button, the user performs the action (create vault, open, unmount, backup), and the tutorial moves on by itself when the vault state changes.

## 2. Abstract

After S009 the dashboard is readable, but a new person does not know why there are two cards, iCloud and a backup, nor in what order to act.

Owner's decisions (2026-11-22):

- **Question at launch:** on every launch, with a "Don't ask at launch" checkbox; changeable in Settings.
- **Introduction:** simple, 5–6 screens, no math.
- **Bubbles on a real vault:** actions lead through the tutorial.

## 3. Objective

- **Goals:**
  - T-01 **Question at launch.** The window opens with a "Tutorial?" sheet: [Start tutorial] [Not now] and a "Don't ask at launch" checkbox. The setting is kept in `UserDefaults`.
  - T-02 **Settings** (⌘, and an item in the padlock menu): an "Ask about the tutorial at launch" toggle and a "Start tutorial now" button.
  - T-03 **Introduction:** 6 screens (the problem, the idea, where the halves live, copies, rules, honest limits of the protection) with an icon, a title and a short text; Back, Next, Skip introduction buttons.
  - T-04 **Guidance.** Steps anchored with a bubble next to a UI element:

    | Step | Completion |
    | --- | --- |
    | create a vault | automatically, when the vault exists; skipped if one already exists |
    | status | "Next" |
    | tiles | "Next" |
    | open the vault | automatically, when mounted |
    | drop files in Finder | "Next" |
    | unmount | automatically, when unmounted |
    | make a backup | automatically, when a new READY point appears |
    | recovery | "Next" |
    | menu bar and settings | "Done" |
  - T-05 **Tutorial bar** at the bottom of the window: step number, "Show tip", Back, Next/Skip step, End. A closed bubble does not interrupt the tutorial.
  - T-06 Tutorial hints in the "New vault" and "Backup" sheets (choice of media).
- **Out of scope:** videos and animations, translations, practicing recovery on real data (the step only explains and points at "Verify").
- **Success criteria:** the gates from §7 and a walkthrough of the tutorial by the owner.

## 4. Artefact Inventory

| Artefact | Type | State |
| --- | --- | --- |
| `macos/Sources/XordockCore/Tutorial.swift` | code: screen content, steps, completion conditions, state machine | to create |
| `macos/Sources/XordockUI/Tutorial.swift` | code: introduction screens, bubble, tutorial bar | to create |
| `macos/Sources/XordockApp/*` | code: question at launch, Settings, bubble anchors | to change |

## 5. Functional Domain Changes

- The `Tour` state machine is pure (Core):
  - `start(state)` skips the "create a vault" step when a vault already exists;
  - `update(state)` advances automatically once the condition is met;
  - `next`, `back`, `finish`.
- The "make a backup" step remembers the identifier of the last point at the moment of entering and completes when a different one appears.
- The tutorial works on the vault selected in the window. When the selection changes, the conditions apply to the new selection.
- The tutorial performs nothing by itself. Actions (writes to media) are done by the user, with the same buttons and sheets as usual (S004 §7).
- Settings: `askTutorialAtLaunch` (default `true`).

## 6. Critical Path

1. Core with tests.
2. Views for the introduction, the bubble and the bar.
3. Anchors in the dashboard and in the list, question at launch, Settings.
4. Renders of the screens to PNG, building the `.app`.

## 7. Control Gates

| # | Gate | Condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | State machine | skipping "create a vault", automatic transitions (mount, unmount, new point), Back/Next/Finish | `swift test` | commit |
| G2 | Content | 6 screens; title ≤ 6 words, text ≤ 320 characters; every step has a title and a text ≤ 220 characters | `swift test` | commit |
| G3 | **Negative:** the tutorial does not act by itself | Core does not call commands; no `Commands.` in the tutorial code | source test | commit |
| G4 | Setting | "Don't ask at launch" saved and reversible in Settings | review + E2E | release |
| G5 | Appearance | renders of the introduction screens and the bubble (light/dark) without truncated texts | review | commit |

## 8. Expected Outcome

A new person understands the point of the split and of the copies, and creates the first vault with a backup by following the bubbles. Rollback: turning off the question in Settings.

## 9. Additional Information

- **Bubble anchors:** native SwiftUI `popover`, which works in the list column, in the dashboard and in the toolbar. Clicking outside the bubble closes it, and the tutorial continues (the bar has "Show tip").
- **Menu bar:** a bubble cannot be anchored to the menu bar icon, so the last step describes it in words.

## Implementation (2026-11-22)

- **Core:** `XordockCore/Tutorial.swift`: `Intro.pages` (6 screens; rules as a list), `TourStep` (9 steps with an anchor, title, text and an "automatic" flag), the `Tour` machine.
- **UI:** `XordockUI/Tutorial.swift`:
  - `TourController` (`@Observable`, shared by the window and the sheets);
  - `TourBubble`, the `.tourAnchor(…)` modifier (native `popover`), `TourBar`, `TourHint`, `IntroView`, `StartPrompt`.
  - Closing the bubble by clicking beside it only hides it ("Show tip" in the bar).
- **Anchors:**
  - "New vault…" in the list;
  - the status card, the tiles;
  - "Open vault", "Unmount", "Backup…", "Recovery…". The main button gets the "mount" or "backup" anchor depending on the recommendation, so the bubble lands on the visible button.
- **Launch:**
  - the menu bar icon (drawn from launch) opens the window when `askTutorialAtLaunch` = true; the window shows `StartPrompt`;
  - padlock menu: "Tutorial…" and "Settings…" (`SettingsLink`, ⌘,);
  - Settings: the toggle and "Start tutorial now".
- **Tests:** `tourFlow` (G1), `tutorialTexts` (G2), `tutorialRunsNoCommands` (G3). 16 Swift tests in total.
- **G5:** renders of the 6 screens, the bubbles and the question in both modes (`XordockPreview`). Examples in `docs/ui/`. After review the "Five rules" screen was changed into a list.
