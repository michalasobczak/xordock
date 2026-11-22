# xordock threat model

State as of 2026-11-22 (S004). Source: S004 §5.7 and §9, analyses `docs/security/2026-11-22-round-S001.md` and `2026-11-22-round-S004.md`.

## What we protect

File contents and their names (names and directory structure are in the XOR-split index).

## Against whom

| Adversary | Protected? |
|---|---|
| Has a full copy of share A (SSD, Time Machine copy, stolen disk), including many historical copies | **Yes**, practically (macOS CSPRNG `SecRandomCopyBytes`). This is **not** an information-theoretic guarantee nor resistance to P = NP (G-09, CG-14). |
| Has only the working card or only the spare card (share B) | **Yes.** B is masks only. |
| Has A and any B card (working or spare) | **No** (out of scope). The spare card is a copy of the same share B, not a separate key (B-02); the working card also holds the snapshot masks (R4-06). |
| Controls the Mac while the vault is mounted (malware, administrator) | **No** (out of scope, I-05). Other local accounts have no access to the volume (macFUSE without `allow_other`). |
| Modifies one share to change the content | **Not detected** (F-03, R-04). Accidental corruption is detected. |
| Has the SSD of a powered-off Mac with FileVault | Protected by FileVault and xordock. Without FileVault, temporary files and application caches may contain plaintext fragments (FR-25). |

## What we do not hide

Sizes and number of objects, write times, the vault name (including in the mount point), the number and dates of recovery points, the fact that xordock is used (D-04, F-05).

## System boundaries (FR-25, IT-03)

Plaintext leaves the vault while working: RAM of applications and of the macFUSE FSKit extension, temporary files and application autosave, QuickLook thumbnails, the Spotlight index, crash dumps, Time Machine and iCloud for copies made by applications, recent documents history (I-04, I-06). Details and recommendations: `docs/usage.md`.
