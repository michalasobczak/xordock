import Foundation
import Testing
@testable import XordockCore

// S007 G2: CLI contract decoding, reminder rules and command arguments.

let vaultsJSON = """
{
  "fileVault": true,
  "masks": "CSPRNG macOS (SecRandomCopyBytes): ochrona praktyczna",
  "vaults": [
    {"name": "test", "id": "517f", "a": "/Users/u/Library/Application Support/xordock/test",
     "card": "/Volumes/xor_primary/xordock/test",
     "spares": [{"root": "/Volumes/xor_backup/xordock/test", "volume": "/Volumes/xor_backup", "media": "m1"}],
     "mounted": true, "mountpoint": "/Volumes/xordock-test", "seq": 25, "snapshots": 1,
     "lastBackup": {"time": "2026-11-22T17:58:00Z", "point": "p1", "snapshot": "s1", "seq": 20, "media": "m1"},
     "changesSinceBackup": 3, "backupDue": false, "newField": 1,
     "aCopy": {"name": "test", "id": "517f", "path": "/icloud/xordock/test", "points": 1, "lastPoint": "2026-11-22T17:58:00Z"}},
    {"name": "old", "error": "share A: corrupted"}
  ]
}
"""

let volumesJSON = """
[
  {"path": "/Volumes/Recovery", "name": "Recovery", "fs": "apfs", "free": 1, "total": 2, "readOnly": false, "systemDisk": true, "vaults": []},
  {"path": "/Volumes/xor_backup", "name": "xor_backup", "fs": "hfs", "free": 100, "total": 200, "readOnly": false, "systemDisk": false,
   "vaults": [{"name": "test", "id": "517f", "role": "backup", "root": "/Volumes/xor_backup/xordock/test"}]},
  {"path": "/Volumes/xor_primary", "name": "xor_primary", "fs": "hfs", "free": 100, "total": 200, "readOnly": false, "systemDisk": false,
   "vaults": [{"name": "test", "id": "517f", "role": "", "root": "/Volumes/xor_primary/xordock/test"}]},
  {"path": "/Volumes/CD", "name": "CD", "fs": "cd9660", "free": 0, "total": 2, "readOnly": true, "systemDisk": false, "vaults": []},
  {"path": "/Volumes/other", "name": "other"}
]
"""

@Test func decodesVaults() throws {
    let s = try jsonDecoder.decode(AppState.self, from: Data(vaultsJSON.utf8))
    #expect(s.vaults.count == 2)
    let v = s.vaults[0]
    #expect(v.mounted && v.seq == 25 && v.changesSinceBackup == 3)
    #expect(v.cardVolume == "/Volumes/xor_primary")
    #expect(v.lastBackup?.point == "p1")
    #expect(v.lastBackup?.time == ISO8601DateFormatter().date(from: "2026-11-22T17:58:00Z"))
    #expect(s.vaults[1].error != nil && s.vaults[1].backupDue && s.vaults[1].card == nil)
    #expect(s.vaults[1].mountpoint == "/Volumes/xordock-old")
}

@Test func attentionRules() throws {
    var s = try jsonDecoder.decode(AppState.self, from: Data(vaultsJSON.utf8))
    #expect(s.attention == .warning) // the second vault has an error
    s.vaults.removeLast()
    #expect(s.attention == .mounted)
    s.vaults[0].mounted = false
    #expect(s.attention == .ok)
    s.vaults[0].backupDue = true
    #expect(s.attention == .warning)
    s.vaults[0].card = nil // without a card we don't force the backup warning
    #expect(s.attention == .ok)
    s.fileVault = false
    #expect(s.attention == .warning)
}

@Test func spareSelection() throws {
    let s = try jsonDecoder.decode(AppState.self, from: Data(vaultsJSON.utf8))
    let vols = try jsonDecoder.decode([VolumeInfo].self, from: Data(volumesJSON.utf8))
    let v = s.vaults[0]
    let cands = v.candidatesForSpare(in: vols).map(\.name)
    // no system disk, no read-only, and not this vault's working card
    #expect(cands == ["xor_backup", "other"])
    #expect(v.defaultSpare(in: vols)?.name == "xor_backup")
    #expect(v.spareVolumes(in: vols).map(\.name) == ["xor_backup"])
}

@Test func commandArgumentsAreArrays() {
    #expect(Commands.initVault(name: "a b", card: "/Volumes/X") == ["init", "--name", "a b", "--card", "/Volumes/X"])
    #expect(Commands.snapshot(name: "t") == ["snapshot", "create", "--name", "t"])
    #expect(Commands.export(name: "t", spare: "/Volumes/S", point: nil, to: "/tmp/x")
        == ["recover", "--name", "t", "--spare", "/Volumes/S", "--to", "/tmp/x", "export"])
    #expect(Commands.restore(name: "t", spare: "/Volumes/S", point: "p", card: "/Volumes/N", newName: "t2")
        == ["recover", "--name", "t", "--spare", "/Volumes/S", "--point", "p", "--card", "/Volumes/N", "--new-name", "t2", "restore"])
}

@Test func vaultNameValidation() {
    for bad in ["", "..", ".x", "a/b", "a:b", " a", "a ", "a\nb", String(repeating: "x", count: 65)] {
        #expect(VaultName.validate(bad) != nil, "accepted \(bad)")
    }
    for good in ["test", "My documents", "résumé-2026"] {
        #expect(VaultName.validate(good) == nil)
    }
}

@Test func healthChangesPlural() throws {
    var v = try sampleVault()
    v.changesSinceBackup = 1
    #expect(VaultHealth.changes(v) == "1 change since the last backup.")
    v.changesSinceBackup = 5
    #expect(VaultHealth.changes(v) == "5 changes since the last backup.")
    v.changesSinceBackup = 0
    #expect(VaultHealth.changes(v) == nil)
}

@Test func stderrCleanup() {
    let raw = "fuse: forking a threaded process is unsafe, the child may crash or deadlock\nxordock: card not found\n"
    #expect(CLI.cleanStderr(raw) == "card not found")
}

// S007 G4, G5: Swift code uses no shell and doesn't read vault directories.
@Test func noShellNoShareAccess() throws {
    let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        .appendingPathComponent("Sources")
    let files = FileManager.default.enumerator(at: root, includingPropertiesForKeys: nil)!
        .compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }
    #expect(!files.isEmpty)
    for f in files {
        let s = try String(contentsOf: f, encoding: .utf8)
        for banned in ["/bin/sh", "/bin/bash", "/bin/zsh", "\"-c\"", "Application Support/xordock", "contentsOfDirectory"] {
            #expect(!s.contains(banned), "\(f.lastPathComponent) contains \(banned)")
        }
    }
}

// Integration with the real CLI, if built (bin/xordock).
@Test func realCLIVolumesDecode() async throws {
    let bin = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        .deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("bin/xordock")
    guard FileManager.default.isExecutableFile(atPath: bin.path) else { return }
    let vols = try await CLI(binary: bin).query([VolumeInfo].self, Commands.volumes())
    #expect(vols.allSatisfy { $0.path.hasPrefix("/Volumes/") })
    do {
        _ = try await CLI(binary: bin).run(["status", "--name", "../x"])
        Issue.record("CLI accepted a name with ../")
    } catch let e as CLIError {
        #expect(e.status != 0)
    }
}

// R7-03: inside an .app bundle XORDOCK_CLI doesn't swap the engine.
@Test func bundledAppIgnoresEnvOverride() throws {
    let tmp = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let app = tmp.appendingPathComponent("x.app/Contents/Resources")
    try FileManager.default.createDirectory(at: app, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: tmp) }
    let evil = tmp.appendingPathComponent("evil")
    FileManager.default.createFile(atPath: evil.path, contents: Data("#!/bin/true\n".utf8), attributes: [.posixPermissions: 0o755])
    let bundle = try #require(Bundle(path: tmp.appendingPathComponent("x.app").path))
    #expect(CLI.locate(bundle: bundle, environment: ["XORDOCK_CLI": evil.path], cwd: tmp.path) == nil)
    FileManager.default.createFile(atPath: app.appendingPathComponent("xordock").path, contents: Data(), attributes: [.posixPermissions: 0o755])
    #expect(CLI.locate(bundle: bundle, environment: ["XORDOCK_CLI": evil.path], cwd: tmp.path)?.binary.lastPathComponent == "xordock")
}

// S008: copy of A in the state, orphaned copies, recovery-from-iCloud arguments.
@Test func aCopyStateAndArgs() throws {
    let json = """
    {"fileVault": true, "masks": "", "icloud": true,
     "vaults": [{"name": "test", "id": "517f", "card": "/Volumes/xor_primary/xordock/test", "backupDue": false,
                 "lastBackup": {"time": "2026-11-22T17:58:00Z", "point": "p1", "snapshot": "s1", "seq": 20, "media": "m1"},
                 "aCopy": {"name": "test", "id": "517f", "path": "/x", "points": 1, "lastPoint": "2026-11-22T17:58:00Z"}}],
     "orphanCopies": []}
    """
    var s = try jsonDecoder.decode(AppState.self, from: Data(json.utf8))
    #expect(s.vaults[0].hasACopy && s.attention == .ok)
    s.vaults[0].aCopy = nil // backup of B without a copy of A → warning
    #expect(s.attention == .warning)
    s.vaults[0].aCopy = CopyInfo(name: "test", id: "517f", path: "/x", points: 1, lastPoint: nil)
    s.orphanCopies = [CopyInfo(name: "old", id: "99", path: "/y", points: 1, lastPoint: nil)]
    #expect(s.attention == .warning)
    let vols = try jsonDecoder.decode([VolumeInfo].self, from: Data(volumesJSON.utf8))
    #expect(recoverySources(id: "517f", volumes: vols, fromICloud: false).map(\.name) == ["xor_backup"])
    #expect(recoverySources(id: "517f", volumes: vols, fromICloud: true).map(\.name) == ["xor_backup", "xor_primary"])
    #expect(Commands.points(name: "t", spare: "/Volumes/S", fromICloud: true) == ["points", "--name", "t", "--spare", "/Volumes/S", "--from-icloud", "--json"])
    #expect(Commands.verify(name: "t", spare: "/Volumes/S", point: nil, fromICloud: true)
        == ["recover", "--name", "t", "--spare", "/Volumes/S", "--from-icloud", "verify"])
}

// S009 G1–G3: dashboard rules, tiles, text brevity.
func sampleVault() throws -> VaultInfo {
    try jsonDecoder.decode(AppState.self, from: Data(vaultsJSON.utf8)).vaults[0]
}

@Test func healthPriorities() throws {
    var v = try sampleVault()
    v.mounted = false
    #expect(VaultHealth.evaluate(v, icloud: true).title == "All good")
    #expect(VaultHealth.evaluate(v, icloud: true).detail == "3 changes since the last backup.")
    v.mounted = true
    #expect(VaultHealth.evaluate(v, icloud: true).action == .showInFinder)
    v.aCopy = nil
    #expect(VaultHealth.evaluate(v, icloud: true).title == "No iCloud copy of A")
    #expect(VaultHealth.evaluate(v, icloud: false).title == "Vault open") // without iCloud we don't require a copy of A
    v.backupDue = true
    #expect(VaultHealth.evaluate(v, icloud: true).title == "Backup older than a week")
    v.lastBackup = nil
    #expect(VaultHealth.evaluate(v, icloud: true).title == "No backup")
    #expect(VaultHealth.evaluate(v, icloud: true).action == .backup)
    v.card = nil
    #expect(VaultHealth.evaluate(v, icloud: true).title == "Connect the working card")
    v.error = "share A: corrupted"
    #expect(VaultHealth.evaluate(v, icloud: true).level == .error)
}

@Test func tilesStates() throws {
    var v = try sampleVault()
    var t = v.tiles(icloud: true)
    #expect(t.map(\.kind) == [.ssd, .card, .spare, .icloud])
    #expect(t.allSatisfy { $0.level == .ok })
    #expect(t[1].note == "xor_primary" && t[2].note == "xor_backup connected")
    v.card = nil
    v.spares = []
    v.aCopy = nil
    v.lastBackup = nil
    t = v.tiles(icloud: true)
    #expect(t[1].status == "not connected" && t[1].level == .warning)
    #expect(t[2].status == "no backup" && t[2].note == nil)
    #expect(t[3].status == "no copy")
    #expect(v.tiles(icloud: false)[3].level == .neutral)
}

@Test func healthTextsAreShort() throws {
    var v = try sampleVault()
    var all: [VaultHealth] = []
    for step in 0..<7 {
        switch step {
        case 1: v.mounted = false
        case 2: v.aCopy = nil
        case 3: v.backupDue = true
        case 4: v.lastBackup = nil
        case 5: v.card = nil
        case 6: v.error = String(repeating: "error ", count: 60)
        default: break
        }
        all.append(VaultHealth.evaluate(v, icloud: true))
    }
    #expect(Set(all.map(\.title)).count == 7)
    for h in all {
        #expect(h.title.split(separator: " ").count <= 5, "\(h.title)")
        #expect(h.detail.count <= 120, "\(h.detail)")
    }
}

// S010 G1: tutorial state machine.
@Test func tourFlow() throws {
    var v = try sampleVault()
    v.mounted = false
    var t = Tour()
    t.start(nil)
    #expect(t.step == .createVault)
    do { let c = t.update(nil); #expect(!c) }
    do { let c = t.update(v); #expect(c && t.step == .status) } // vault created
    t.back(v)
    #expect(t.step == .status) // don't go back to "create vault" when a vault exists
    t.next(v); #expect(t.step == .tiles)
    t.next(v); #expect(t.step == .mount)
    do { let c = t.update(v); #expect(!c) }
    v.mounted = true
    do { let c = t.update(v); #expect(c && t.step == .useFinder) }
    do { let c = t.update(v); #expect(!c) } // manual step
    t.next(v); #expect(t.step == .unmount)
    v.mounted = false
    do { let c = t.update(v); #expect(c && t.step == .backup) }
    do { let c = t.update(v); #expect(!c) } // same point as on entry
    v.lastBackup?.point = "new"
    do { let c = t.update(v); #expect(c && t.step == .recovery) }
    t.next(v); #expect(t.step == .menuBar)
    t.next(v); #expect(!t.active)

    var u = Tour()
    u.start(v)
    #expect(u.step == .status && u.number == 2)
    u.finish()
    #expect(!u.active)
}

// S010 G2: concise content.
@Test func tutorialTexts() {
    #expect(Intro.pages.count == 6)
    for p in Intro.pages {
        #expect(p.title.split(separator: " ").count <= 6, "\(p.title)")
        #expect(p.text.count <= 320, "\(p.title): \(p.text.count)")
    }
    for s in TourStep.allCases {
        #expect(!s.title.isEmpty && s.text.count <= 220, "\(s): \(s.text.count)")
    }
}

// S010 G3: the tutorial runs no commands.
@Test func tutorialRunsNoCommands() throws {
    let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    for rel in ["Sources/XordockCore/Tutorial.swift", "Sources/XordockUI/Tutorial.swift"] {
        let f = root.appendingPathComponent(rel)
        guard let s = try? String(contentsOf: f, encoding: .utf8) else { continue }
        #expect(!s.contains("Commands.") && !s.contains("CLI("), "\(rel)")
    }
}

// S011 G4: doctor result and launch rules.
@Test func requirementsRules() throws {
    func result(_ macfuse: String, _ ext: String) throws -> DoctorResult {
        let json = """
        {"macfuseVersion": "5.5.0", "checks": [
          {"id": "macos", "title": "macOS", "level": "ok", "detail": ""},
          {"id": "macfuse", "title": "macFUSE", "level": "\(macfuse)", "detail": "", "fixLabel": "Download", "fixURL": "https://macfuse.github.io/"},
          {"id": "extension", "title": "Extension", "level": "\(ext)", "detail": ""},
          {"id": "icloud", "title": "iCloud", "level": "warning", "detail": ""}]}
        """
        return try jsonDecoder.decode(DoctorResult.self, from: Data(json.utf8))
    }
    let unknown = try result("ok", "unknown")
    #expect(!unknown.hasErrors) // the iCloud warning doesn't block
    #expect(unknown.needsProbe(confirmedFor: nil))
    #expect(unknown.needsProbe(confirmedFor: "5.4.0")) // new macFUSE version → probe again
    #expect(!unknown.needsProbe(confirmedFor: "5.5.0"))
    #expect(unknown.applyingConfirmation("5.5.0").checks[2].level == "ok")
    #expect(unknown.applyingConfirmation("5.4.0").checks[2].level == "unknown")
    let missing = try result("error", "unknown")
    #expect(missing.hasErrors && !missing.needsProbe(confirmedFor: nil))
    #expect(missing.checks[1].fixURL == "https://macfuse.github.io/")
    #expect(try result("ok", "ok").confirmedVersion == "5.5.0")
    #expect(try result("ok", "error").hasErrors)
    #expect(Commands.doctor(probe: true) == ["doctor", "--json", "--probe-mount"])
}

// The app is English only: no Polish letters or Polish quotes anywhere in
// macos/Sources and macos/Tests, comments included. The banned characters are
// written as Unicode escapes so this file passes its own check.
@Test func sourcesAreEnglishOnly() throws {
    let macos = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    let banned: Set<Character> = [
        "\u{0105}", "\u{0107}", "\u{0119}", "\u{0142}", "\u{0144}", "\u{00F3}", "\u{015B}", "\u{017A}", "\u{017C}",
        "\u{0104}", "\u{0106}", "\u{0118}", "\u{0141}", "\u{0143}", "\u{00D3}", "\u{015A}", "\u{0179}", "\u{017B}",
        "\u{201E}", "\u{201D}",
    ]
    var count = 0
    for dir in ["Sources", "Tests"] {
        let files = FileManager.default.enumerator(at: macos.appendingPathComponent(dir), includingPropertiesForKeys: nil)!
            .compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }
        count += files.count
        for f in files {
            let s = try String(contentsOf: f, encoding: .utf8)
            for (i, line) in s.components(separatedBy: "\n").enumerated() {
                #expect(!line.contains(where: { banned.contains($0) }), "\(dir)/\(f.lastPathComponent):\(i + 1): \(line)")
            }
        }
    }
    #expect(count > 1)
}
