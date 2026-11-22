import Foundation

// UI-independent app rules (S007 §5), tested in XordockCoreTests.

public enum VaultName {
    /// The vault name becomes a path component (A and B), so it must not contain
    /// a separator or start with a dot. The same rule is in the CLI.
    public static func validate(_ s: String) -> String? {
        let t = s
        if t.isEmpty { return "Enter a vault name." }
        if t.count > 64 { return "The name can be at most 64 characters." }
        if t.hasPrefix(".") { return "The name can't start with a dot." }
        if t.contains("/") || t.contains(":") || t.unicodeScalars.contains(where: { $0.value < 0x20 }) {
            return "The name can't contain \"/\", \":\" or control characters."
        }
        if t != t.trimmingCharacters(in: .whitespaces) { return "The name can't start or end with a space." }
        return nil
    }
}

/// CLI command arguments. Always an array, never a shell (S007 G4).
public enum Commands {
    public static func vaults() -> [String] { ["vaults", "--json"] }
    public static func volumes() -> [String] { ["volumes", "--json"] }
    public static func initVault(name: String, card: String) -> [String] { ["init", "--name", name, "--card", card] }
    public static func mount(name: String) -> [String] { ["mount", "--name", name] }
    public static func unmount(name: String) -> [String] { ["unmount", "--name", name] }
    public static func snapshot(name: String) -> [String] { ["snapshot", "create", "--name", name] }
    public static func backup(name: String, spare: String) -> [String] { ["backup", "--name", name, "--spare", spare] }
    public static func fsck(name: String) -> [String] { ["fsck", "--name", name] }
    public static func points(name: String, spare: String, fromICloud: Bool = false) -> [String] {
        ["points", "--name", name, "--spare", spare] + (fromICloud ? ["--from-icloud"] : []) + ["--json"]
    }

    public static func recover(_ sub: String, name: String, spare: String, point: String?, fromICloud: Bool = false, extra: [String] = []) -> [String] {
        var a = ["recover", "--name", name, "--spare", spare]
        if fromICloud { a.append("--from-icloud") }
        if let p = point, !p.isEmpty { a += ["--point", p] }
        return a + extra + [sub]
    }
    public static func verify(name: String, spare: String, point: String?, fromICloud: Bool = false) -> [String] {
        recover("verify", name: name, spare: spare, point: point, fromICloud: fromICloud)
    }
    public static func export(name: String, spare: String, point: String?, to: String, fromICloud: Bool = false) -> [String] {
        recover("export", name: name, spare: spare, point: point, fromICloud: fromICloud, extra: ["--to", to])
    }
    public static func restore(name: String, spare: String, point: String?, card: String, newName: String, fromICloud: Bool = false) -> [String] {
        recover("restore", name: name, spare: spare, point: point, fromICloud: fromICloud, extra: ["--card", card, "--new-name", newName])
    }
}

public enum Attention: Equatable, Sendable {
    case ok, mounted, warning
}

public extension AppState {
    /// Menu bar icon state: warning when any vault has an error or
    /// needs a backup (no READY point or older than 7 days, S004 B-01).
    var attention: Attention {
        if !fileVault || !orphanCopies.isEmpty
            || vaults.contains(where: { $0.error != nil || ($0.card != nil && ($0.backupDue || ($0.lastBackup != nil && !$0.hasACopy))) }) {
            return .warning
        }
        if vaults.contains(where: { $0.mounted }) { return .mounted }
        return .ok
    }
}

public extension VaultInfo {
    /// Default spare card: the volume that already holds the vault's spare card.
    func defaultSpare(in volumes: [VolumeInfo]) -> VolumeInfo? {
        let known = Set(spares.map(\.volume))
        return candidatesForSpare(in: volumes).first { known.contains($0.path) }
    }

    /// Drives allowed as backup targets: writable, not system disks,
    /// not this vault's working card.
    func candidatesForSpare(in volumes: [VolumeInfo]) -> [VolumeInfo] {
        volumes.filter { $0.usableAsCard && $0.path != cardVolume && !$0.vaults.contains { $0.id == id && !$0.isSpare } }
    }

    /// Drives holding this vault's spare card (for recovery).
    func spareVolumes(in volumes: [VolumeInfo]) -> [VolumeInfo] {
        recoverySources(id: id, volumes: volumes, fromICloud: false)
    }

    /// There is a copy of share A in iCloud with at least one READY point.
    var hasACopy: Bool { (aCopy?.points ?? 0) > 0 }

    var statusLine: String {
        if let e = error { return "Error: \(e)" }
        if card == nil { return "Working card not connected" }
        if mounted { return "Mounted" }
        return "Ready to mount"
    }
}

/// Share B sources for recovery: the vault's spare card and, with A from iCloud
/// (SSD lost), also the working card (S008 K-04).
public func recoverySources(id: String, volumes: [VolumeInfo], fromICloud: Bool) -> [VolumeInfo] {
    volumes.filter { $0.vaults.contains { $0.id == id && ($0.isSpare || fromICloud) } }
}

public func formatBytes(_ n: UInt64) -> String {
    ByteCountFormatter.string(fromByteCount: Int64(clamping: n), countStyle: .file)
}
