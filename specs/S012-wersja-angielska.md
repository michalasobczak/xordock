# S012 — English version of the project

- **Status:** delivered
- **Date:** 2026-11-22
- **Related:** S007–S011, CLAUDE.md (project language rule)

## 1. TL;DR

All project text changes from Polish to English: user-facing texts in the macOS app and the CLI, README, CHANGELOG, MANIFEST, documentation, all specs and code comments (Go and Swift). The Polish texts disappear; there is no bilingual version and no localization tables. Tests guard against Polish texts returning to production code, and a grep check covers documents and comments.

## 2. Abstract

Until now the interface and all artifacts were in Polish, following the project rule "artifacts and communication in Polish".

The owner asked (2026-11-22) for an English version: "just throw out the Polish texts". Asked about the scope, the owner first chose the **app + CLI** variant:

- window, menus, tutorial, requirements;
- CLI messages and errors, which the app shows unchanged.

Later the same day (2026-11-22) the owner extended the decision: the scope is now **everything**:

- app and CLI texts;
- `README.md`, `CHANGELOG.md`, `MANIFEST.md`;
- documentation in `docs/`;
- all specs in `specs/` (including the template);
- code comments in Go and Swift.

The project rule in `CLAUDE.md` changes to "artifacts in English, conversation with the owner in Polish". File names are not renamed, so Polish words may remain in paths.

## 3. Objective

- **Goals:**
  - E-01 All user-facing text literals in `cmd/`, `internal/` and `macos/Sources/` in English: usage, output, errors, logs, `doctor`, mask description, UI, tutorial, requirements.
  - E-02 One glossary of terms: vault, share A/B, working card, spare card, snapshot, READY recovery point, share A copy in iCloud Drive, mount/unmount, recovery, export, restore.
  - E-03 Dates in the app in `en_US` format; English plurals.
  - E-04 Security warnings keep their exact meaning:
    - practical protection, without an information-theoretic guarantee;
    - the export is plaintext;
    - iCloud account + any B card = a complete set;
    - cards kept apart;
    - unmount before removal;
    - 2FA and ADP.
  - E-05 `README.md`, `CHANGELOG.md`, `MANIFEST.md`, `docs/**` and `specs/**` (including `SZABLON.md`) translated faithfully and completely into English, keeping structure, IDs, dates, commands, paths and links; the E-02 glossary applies.
  - E-06 Code comments in Go (`cmd/`, `internal/`) and Swift (`macos/`) in English.
  - E-07 `CLAUDE.md`: the rule "artifacts and communication in Polish" replaced by "artifacts in English, conversation with the owner in Polish".
- **Out of scope:**
  - localization (multiple languages);
  - bilingual documents;
  - renaming spec files (`specs/SNNN-<polish-name>.md` names stay; `docs/` was renamed on the owner's request, see Implementation);
  - the language of conversation with the owner (stays Polish).
- **Success criteria:** the gates from §7.

## 4. Artefact Inventory

| Artefact | Type | State |
| --- | --- | --- |
| `cmd/xordock/*.go`, `internal/**/*.go` | code: texts and comments | to change |
| `cmd/xordock/english_test.go` | test: no Polish characters in Go literals (go/ast) | to create |
| `macos/Sources/**`, `macos/Tests/**` | code and tests: texts and comments | to change |
| `docs/ui/*.png` | renders | to refresh |
| `README.md`, `CHANGELOG.md`, `MANIFEST.md` | documents | to change |
| `docs/**` | documentation | to change |
| `specs/**` | specs (S001–S012, `SZABLON.md`) | to change |
| `CLAUDE.md` | project instructions: language rule | to change |

## 5. Functional Domain Changes

- Behavior and data format unchanged. Only texts change, plus text comparisons in tests.
- The project language rule in `CLAUDE.md` changes, by the owner's decision, to "artifacts in English, conversation with the owner in Polish". This is no longer a deviation from the rule but the new rule itself.
- Historical entries (CHANGELOG, "Implementation" sections of specs, security reports) are translated, not rewritten: their content and dates stay the same.

## 6. Critical Path

1. Translation of Go and Swift texts (in parallel, shared glossary).
2. Guard tests.
3. Translation of code comments.
4. Translation of README, CHANGELOG, MANIFEST, docs and specs; change of the rule in `CLAUDE.md`.
5. Renders and building the `.app`.
6. Consistency review.

## 7. Control Gates

| # | Gate | Condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | Go without Polish literals | no text literal in production code contains Polish characters or Polish quotation marks (U+201E, U+201D) | `TestSourcesAreEnglish` (go/ast) | commit |
| G2 | Swift without Polish texts | no code line (outside comments) in `macos/Sources` contains Polish characters | Swift test | commit |
| G3 | No regressions | `go test -race ./...`, `swift test` green | test | commit |
| G4 | Consistency | E-02 glossary used consistently; E-04 warnings present | review + grep | commit |
| G5 | Appearance | renders of the dashboard, tutorial and requirements without truncated texts | review | commit |
| G6 | Docs and specs in English | `README.md`, `CHANGELOG.md`, `MANIFEST.md`, `docs/**`, `specs/**` contain no Polish text, except file names inside paths | grep for Polish characters + review | commit |
| G7 | Code comments in English | Go and Swift comments contain no Polish text | grep for Polish characters + review | commit |
| G8 | Project rule | `CLAUDE.md` says "artifacts in English, conversation with the owner in Polish" | review | commit |

## 8. Expected Outcome

The app, the CLI, the code and all project artifacts are entirely in English; conversation with the owner stays in Polish. Rollback: `git revert`.

## 9. Additional Information

- The guards detect only Polish diacritics. Words without them (e.g. "sejf") are checked by a grep review.
- Polish file names (e.g. `specs/S012-wersja-angielska.md`, `docs/ui/requirements-light.png`) are kept so that links and history stay valid; the G6 grep tolerates them only inside paths.

## Implementation (2026-11-22)

- Translation done in parallel by five agents sharing one glossary (Go texts and comments; Swift texts and comments; README/CHANGELOG/MANIFEST/docs/template; specs S001–S006; specs S007–S012), then reviewed: Polish-character scan of every tracked text file, UI labels quoted in docs compared with the app's actual strings, safety warnings checked in CLI and app.
- `docs/` renamed on the owner's request: `docs/uzytkowanie.md` → `docs/usage.md`, `docs/bezpieczenstwo/` → `docs/security/` (`…-runda-S00x.md` → `…-round-S00x.md`), renders in `docs/ui/` (`*-jasny/ciemny` → `*-light/dark`, `dymek-` → `tip-`, `wymagania` → `requirements`, …); all references updated. `XordockPreview` now writes English file names.
- `CLAUDE.md` translated, with the new rule "artifacts in English, conversation with the owner in Polish". `MANIFEST.md` M23 updated the same way. `scripts/build-app.sh` translated (prints `Built: …`).
- Default vault name `sejf` kept for compatibility (part of the default share A/B paths); the restore default is now `<name>-recovered`, the probe volume `xordock-probe`.
- Strengthened during the review: after `recover … restore` the CLI now says to delete **every** copy of the old share A (SSD, iCloud Drive incl. Recently Deleted, Time Machine), matching R8-01 and `docs/recovery.md`.
- Gates:
  - G1, G7 (Go): `TestSourcesAreEnglish` parses all Go files with comments (tests included) and scans `ops_darwin.c`/`bridge.h`;
  - G2, G7 (Swift): `sourcesAreEnglishOnly` scans every line of `macos/Sources` and `macos/Tests`;
  - G3: `go test -race ./...` and `swift test` (19) green;
  - G5: renders reviewed (dashboard, tutorial, requirements; light/dark), nothing truncated;
  - G6: Unicode-aware scan of all tracked text files finds no Polish characters; Polish words remain only in spec file names.
