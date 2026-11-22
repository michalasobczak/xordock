# S003 — Protecting a folder from Finder

- **Status:** draft — owner's request, requires a spec session
- **Date:** 2026-11-22
- **Related:** S001, S002

## 1. TL;DR

The user selects a folder in Finder and chooses "Protect with SD card (xordock)" from the context menu. The folder goes into a vault, and its place is taken by the mounted protected folder. No terminal.

## 2. Abstract

Owner's question (2026-11-22): "how can I select a folder and choose encryption with the SD card as the key, or is there only the cmd option?". In S001 the only route is the CLI (`xordock init/mount`), and the vault starts as an empty folder.

Moving an **existing** folder has an important consequence: the original plaintext files remain in free SSD blocks after deletion. On APFS/SSD they cannot be reliably overwritten (copy-on-write, TRIM). Only FileVault provides protection. The spec must tell the user this explicitly before they agree to the import (G-06, FR-24).

## 3. Objective

- **Goals:** a Finder action (Quick Action / Service) "Protect with SD card"; import of a folder into a new or existing vault; mounting in the same place; "Unmount" and "Mount" actions without the terminal; a menu bar app showing the state of the vault and the card.
- **Out of scope:** [TO BE DECIDED]
- **Success criteria:** [TO BE DECIDED]

## 4. Artefact Inventory

| Artefact | Kind | State |
| --- | --- | --- |
| `xordock import` (CLI) | code | to be created |
| Finder Quick Action (Automator `.workflow` calling the CLI) | code | to be created |
| Menu bar app (mounting in the background, card state) | code | [TO BE DECIDED] (depends on S002: FSKit provides this naturally) |

## 5. Functional Domain Changes

[TO BE DECIDED]. Questions for the session:

1. Import: move (original deleted, but remnants on the SSD) or copy (original stays, the user deletes it themselves)?
2. One vault per card with many folders, or a separate vault for each folder?
3. Should the protected folder be mounted in the place of the original (e.g. `~/Documents/Umowy`), or in `~/xordock/<name>`?
4. Should mounting after inserting the card be automatic?
5. Order relative to S002: the Finder menu can be done right away on WebDAV (Automator, no Xcode); the menu bar app is better after the FSKit decision.

## 6–9.

To be completed in the spec session.
