# How to use xordock safely

> **Prototype.** Do not store real secret documents before the failure and recovery gates have been passed on your own hardware. Masks come from the macOS CSPRNG: practical (computational) protection, **NO** information-theoretic guarantee and no resistance to P = NP.

## First run

Requirements: macFUSE 5.5+ (`brew install --cask macfuse`) and the extension enabled: System Settings → General → Login Items & Extensions → File System Extensions → macFUSE. No kext, no lowering of security settings.

```bash
go build -o bin/xordock ./cmd/xordock
./bin/xordock init --card /Volumes/xordock --name sejf
./bin/xordock mount --name sejf          # volume /Volumes/xordock-sejf in Finder
```

The mount lasts until the volume is ejected in Finder, `xordock unmount --name sejf` or Ctrl-C. Removing the card while working locks the volume (EIO errors) and unmounts it within approx. 1 s; uncommitted changes are lost, the last committed version remains. Changes to a file are committed when it is closed or on `fsync`.

Limitations: no permissions (files are 0600), `xattr`, symlinks or hard links; swapping files (`RENAME_SWAP`) returns an error.

- Share A: `~/Library/Application Support/xordock/sejf`
- Share B: `/Volumes/<card>/xordock/sejf`. The card is recognized by the vault identifier, not by the volume name.

## SD card

- **Losing the card without a backup = losing the data.** Take a snapshot and back up to a spare card regularly: [recovery.md](recovery.md). A copy of share A alone (e.g. Time Machine) is **not enough** to recover the data.
- A card taken out of service: overwrite it entirely or destroy it. Simply deleting the files does not remove the masks from the medium (F-04).
- An ordinary SD card can be copied. It is not a hardware token.

## Where plaintext can leak outside the vault (FR-25, FR-26)

xordock does not control other programs. We do not promise that they delete their copies.

- **Autosave and temporary files** of office applications (often in `~/Library/Containers/…` or `/private/var/folders/…`).
- **QuickLook thumbnails and Finder previews** (`~/Library/Caches`, QuickLook cache).
- **Recent documents**: file names in application and system history.
- **RAM and swap.** Swap on Apple Silicon is encrypted.
- **Spotlight, crash dumps, Time Machine, iCloud**, if applications save copies outside the volume.

Recommendations: FileVault enabled (FR-27), open files from the vault in the simplest possible programs, unmount the folder after work.

## Diagnostics and exit

```bash
./bin/xordock fsck --name sejf
./bin/xordock export --name sejf --to ~/Desktop/eksport-sejfu
```

`fsck` checks the checksums of both shares of every object without reassembling the content. `export` (rollback from S001 §8) writes plaintext files to a new directory. These files are **no longer protected**.
