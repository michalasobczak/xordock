import Foundation
import Testing
@testable import XordockCore

// S007 G2: dekodowanie kontraktu CLI, reguły przypomnień i argumenty poleceń.

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
     "changesSinceBackup": 3, "backupDue": false, "nowePole": 1,
     "aCopy": {"name": "test", "id": "517f", "path": "/icloud/xordock/test", "points": 1, "lastPoint": "2026-11-22T17:58:00Z"}},
    {"name": "stary", "error": "udział A: uszkodzony"}
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
  {"path": "/Volumes/inny", "name": "inny"}
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
    #expect(s.vaults[1].mountpoint == "/Volumes/xordock-stary")
}

@Test func attentionRules() throws {
    var s = try jsonDecoder.decode(AppState.self, from: Data(vaultsJSON.utf8))
    #expect(s.attention == .warning) // drugi sejf ma błąd
    s.vaults.removeLast()
    #expect(s.attention == .mounted)
    s.vaults[0].mounted = false
    #expect(s.attention == .ok)
    s.vaults[0].backupDue = true
    #expect(s.attention == .warning)
    s.vaults[0].card = nil // bez karty nie wymuszamy ostrzeżenia o backupie
    #expect(s.attention == .ok)
    s.fileVault = false
    #expect(s.attention == .warning)
}

@Test func spareSelection() throws {
    let s = try jsonDecoder.decode(AppState.self, from: Data(vaultsJSON.utf8))
    let vols = try jsonDecoder.decode([VolumeInfo].self, from: Data(volumesJSON.utf8))
    let v = s.vaults[0]
    let cands = v.candidatesForSpare(in: vols).map(\.name)
    // bez systemowego, bez tylko-do-odczytu i bez karty roboczej tego sejfu
    #expect(cands == ["xor_backup", "inny"])
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
        #expect(VaultName.validate(bad) != nil, "przyjęto \(bad)")
    }
    for good in ["test", "Moje dokumenty", "zdjęcia-2026"] {
        #expect(VaultName.validate(good) == nil)
    }
}

@Test func stderrCleanup() {
    let raw = "fuse: forking a threaded process is unsafe, the child may crash or deadlock\nxordock: nie znaleziono karty\n"
    #expect(CLI.cleanStderr(raw) == "nie znaleziono karty")
}

// S007 G4, G5: kod Swift nie używa powłoki i nie czyta katalogów sejfów.
@Test func noShellNoShareAccess() throws {
    let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        .appendingPathComponent("Sources")
    let files = FileManager.default.enumerator(at: root, includingPropertiesForKeys: nil)!
        .compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }
    #expect(!files.isEmpty)
    for f in files {
        let s = try String(contentsOf: f, encoding: .utf8)
        for banned in ["/bin/sh", "/bin/bash", "/bin/zsh", "\"-c\"", "Application Support/xordock", "contentsOfDirectory"] {
            #expect(!s.contains(banned), "\(f.lastPathComponent) zawiera \(banned)")
        }
    }
}

// Integracja z prawdziwym CLI, jeśli zbudowany (bin/xordock).
@Test func realCLIVolumesDecode() async throws {
    let bin = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        .deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("bin/xordock")
    guard FileManager.default.isExecutableFile(atPath: bin.path) else { return }
    let vols = try await CLI(binary: bin).query([VolumeInfo].self, Commands.volumes())
    #expect(vols.allSatisfy { $0.path.hasPrefix("/Volumes/") })
    do {
        _ = try await CLI(binary: bin).run(["status", "--name", "../x"])
        Issue.record("CLI przyjął nazwę z ../")
    } catch let e as CLIError {
        #expect(e.status != 0)
    }
}

// R7-03: w pakiecie .app XORDOCK_CLI nie podmienia silnika.
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

// S008: kopia A w stanie, osierocone kopie, argumenty odzyskiwania z iCloud.
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
    s.vaults[0].aCopy = nil // backup B bez kopii A → ostrzeżenie
    #expect(s.attention == .warning)
    s.vaults[0].aCopy = CopyInfo(name: "test", id: "517f", path: "/x", points: 1, lastPoint: nil)
    s.orphanCopies = [CopyInfo(name: "stary", id: "99", path: "/y", points: 1, lastPoint: nil)]
    #expect(s.attention == .warning)
    let vols = try jsonDecoder.decode([VolumeInfo].self, from: Data(volumesJSON.utf8))
    #expect(recoverySources(id: "517f", volumes: vols, fromICloud: false).map(\.name) == ["xor_backup"])
    #expect(recoverySources(id: "517f", volumes: vols, fromICloud: true).map(\.name) == ["xor_backup", "xor_primary"])
    #expect(Commands.points(name: "t", spare: "/Volumes/S", fromICloud: true) == ["points", "--name", "t", "--spare", "/Volumes/S", "--from-icloud", "--json"])
    #expect(Commands.verify(name: "t", spare: "/Volumes/S", point: nil, fromICloud: true)
        == ["recover", "--name", "t", "--spare", "/Volumes/S", "--from-icloud", "verify"])
}

// S009 G1–G3: reguły pulpitu, kafelki, zwięzłość tekstów.
func sampleVault() throws -> VaultInfo {
    try jsonDecoder.decode(AppState.self, from: Data(vaultsJSON.utf8)).vaults[0]
}

@Test func healthPriorities() throws {
    var v = try sampleVault()
    v.mounted = false
    #expect(VaultHealth.evaluate(v, icloud: true).title == "Wszystko w porządku")
    #expect(VaultHealth.evaluate(v, icloud: true).detail == "3 zmiany od ostatniego backupu.")
    v.mounted = true
    #expect(VaultHealth.evaluate(v, icloud: true).action == .showInFinder)
    v.aCopy = nil
    #expect(VaultHealth.evaluate(v, icloud: true).title == "Brak kopii A w iCloud")
    #expect(VaultHealth.evaluate(v, icloud: false).title == "Sejf otwarty") // bez iCloud nie wymagamy kopii A
    v.backupDue = true
    #expect(VaultHealth.evaluate(v, icloud: true).title == "Backup starszy niż tydzień")
    v.lastBackup = nil
    #expect(VaultHealth.evaluate(v, icloud: true).title == "Brak backupu")
    #expect(VaultHealth.evaluate(v, icloud: true).action == .backup)
    v.card = nil
    #expect(VaultHealth.evaluate(v, icloud: true).title == "Podłącz kartę roboczą")
    v.error = "udział A: uszkodzony"
    #expect(VaultHealth.evaluate(v, icloud: true).level == .error)
}

@Test func tilesStates() throws {
    var v = try sampleVault()
    var t = v.tiles(icloud: true)
    #expect(t.map(\.kind) == [.ssd, .card, .spare, .icloud])
    #expect(t.allSatisfy { $0.level == .ok })
    #expect(t[1].note == "xor_primary" && t[2].note == "xor_backup podłączona")
    v.card = nil
    v.spares = []
    v.aCopy = nil
    v.lastBackup = nil
    t = v.tiles(icloud: true)
    #expect(t[1].status == "niepodłączona" && t[1].level == .warning)
    #expect(t[2].status == "brak backupu" && t[2].note == nil)
    #expect(t[3].status == "brak kopii")
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
        case 6: v.error = String(repeating: "błąd ", count: 60)
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
