# S002 — Folder in Finder via FSKit

- **Status:** abandoned (2026-11-22). A custom FSKit extension requires the paid Apple Developer Program; the owner chose S004 (macFUSE with the FSKit backend, without a custom entitlement). The `macos/` project was deleted before commit. G1 and G2 were checked in S004 (E2E, S004 round analysis).
- **Date:** 2026-11-22
- **Related:** S001 (F-01, F-02 in `docs/security/2026-11-22-round-S001.md`), S003

## 1. TL;DR

The vault is visible in Finder as an ordinary volume thanks to a native macOS file system extension (FSKit), with no local network server and no cache files with plaintext content on the SSD. Confirmation: all S001 gates pass through the new volume, and the E2E test finds no test content outside RAM and the shares.

## 2. Abstract

The S001 prototype mounted the vault via local WebDAV. The security analysis showed that the macOS WebDAV agent writes file contents to the SSD (F-01), and the access token is visible in the mount table (F-02).

Decision history (2026-11-22): NFS loopback (client cache in RAM) was considered. The NFSv3 server prototype reached the mount attempt and was stopped. The owner rejected NFS and WebDAV: the integration must be native. The WebDAV and NFS code was removed, so `xordock mount` does not exist. Earlier, FSKit was blocked by the lack of Xcode; the owner installed Xcode 27.2.

Stable file numbers in the vault index (`Entry.Ino`) and number-based operations (`ByIno`, `SetFileIno`, `Create`, `Remove`, `RenameReplacing`) were kept, because FSKit identifies items by stable identifiers.

## 3. Objective

- **Goals:** an FSKit extension (file system module) that mounts the vault, a container app that ships it, card handling as in S001 (SC-02, FR-18).
- **Out of scope:** changing the share format; the "Protect with SD card" action in Finder (S003).
- **Success criteria:** gates CG-01–CG-11 from S001 through the FSKit volume; G1 and G2 below; F-01 and F-02 closed in the next security analysis.

## 4. Artefact Inventory

| Artefact | Kind | State |
| --- | --- | --- |
| Xcode 27.2, macOS SDK 27.2 with FSKit | external | exists |
| Apple Development certificate (Personal Team `F5AKYMU3G6`) + Apple WWDR G3 intermediate | external | exists (2026-11-22), signing works |
| Entitlement `com.apple.developer.fskit.fsmodule` | external | **blocker**: Personal Team does not support it; the paid Apple Developer Program is required |
| `macos/Xordock.xcodeproj`, `macos/Xordock/XordockApp.swift` (container app), `macos/XordockFS/` (read-only "hello" module, Info.plist with `FSSupportsPathURLs`, entitlements) | code | exists, compiles; cannot be signed without a profile |
| `internal/vault` (vault engine in Go) | code | exists |
| Go → Swift bridge (`go build -buildmode=c-archive`) | code | to be created (recommendation, see §9) |
| Xcode project: app + FSKit extension (Swift) | code | to be created |
| Enabling the extension by the user (System Settings) | organization | to be done by the owner |

## 5. Functional Domain Changes

[TO BE DECIDED] in a session. To resolve:

- how the user mounts the vault (menu bar app, `mount -F`, automatically when the card is inserted);
- where in FSKit to buffer writes and when to commit (file close, `fsync`, idle), to preserve FR-10 and FR-11;
- behavior when the card is removed during work.

### Step 1 result (2026-11-22)

`xcodebuild -allowProvisioningUpdates`: *"Personal development teams … do not support the FSKit Module capability."* Without a paid account Apple will not issue a profile with the FSKit entitlement, and macOS will not run the extension without it (a workaround requires disabling SIP/AMFI, which we reject: it lowers the security of the whole Mac, E4).

## 6. Critical Path

1. Signing identity and confirmation that an FSKit extension with the entitlement loads (minimal "hello" module).
2. Bridge to the Go engine and read-only operations.
3. Write, rename, delete, commit.
4. Card: supervision and unmount.
5. E2E tests of the S001 gates, G1, G2; security analysis.

## 7. Control Gates

| # | Gate | Pass condition | Verification | Blocks |
| --- | --- | --- | --- | --- |
| G1 | No plaintext content on the SSD outside RAM | 0 marker hits in files opened by system processes (`lsof`) and in `/private/tmp` and `/private/var/folders` during and after work | E2E | release |
| G2 | No access via network or socket | the extension opens no listening socket | `lsof -i` | release |
| G3 | Gates CG-01–CG-11 from S001 | green | `go test ./...` + E2E | release |

## 8. Expected Outcome

The vault is mounted natively, F-01 and F-02 closed.

## 9. Additional Information

- **Rejected:** WebDAV (F-01, F-02), NFS loopback (owner's decision), macFUSE (kernel extension, lowered security).
- **Bridge recommendation:** the vault engine stays in Go and is linked into the extension as a C library. Rewriting the engine in Swift would produce two implementations of the format (M2: no duplication). A separate helper process over XPC is an additional boundary to secure.
- **Open question:** will the owner join the Apple Developer Program (USD 99/year)? Without it S002 is blocked.
