import Foundation

// Reguły aplikacji niezależne od UI (S007 §5), testowane w XordockCoreTests.

public enum VaultName {
    /// Nazwa sejfu staje się składnikiem ścieżki (A i B), więc nie może zawierać
    /// separatora ani zaczynać się kropką. Ta sama reguła jest w CLI.
    public static func validate(_ s: String) -> String? {
        let t = s
        if t.isEmpty { return "Podaj nazwę sejfu." }
        if t.count > 64 { return "Nazwa może mieć najwyżej 64 znaki." }
        if t.hasPrefix(".") { return "Nazwa nie może zaczynać się kropką." }
        if t.contains("/") || t.contains(":") || t.unicodeScalars.contains(where: { $0.value < 0x20 }) {
            return "Nazwa nie może zawierać „/”, „:” ani znaków sterujących."
        }
        if t != t.trimmingCharacters(in: .whitespaces) { return "Nazwa nie może zaczynać się ani kończyć spacją." }
        return nil
    }
}

/// Argumenty poleceń CLI. Zawsze tablica, nigdy powłoka (S007 G4).
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
    /// Stan ikony w pasku menu: ostrzeżenie, gdy któryś sejf ma błąd albo
    /// wymaga backupu (brak punktu READY lub starszy niż 7 dni, S004 B-01).
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
    /// Domyślna karta zapasowa: wolumin, na którym już jest karta zapasowa sejfu.
    func defaultSpare(in volumes: [VolumeInfo]) -> VolumeInfo? {
        let known = Set(spares.map(\.volume))
        return candidatesForSpare(in: volumes).first { known.contains($0.path) }
    }

    /// Nośniki, na które wolno zrobić backup: zapisywalne, nie systemowe,
    /// nie karta robocza tego sejfu.
    func candidatesForSpare(in volumes: [VolumeInfo]) -> [VolumeInfo] {
        volumes.filter { $0.usableAsCard && $0.path != cardVolume && !$0.vaults.contains { $0.id == id && !$0.isSpare } }
    }

    /// Nośniki z kartą zapasową tego sejfu (do odzyskiwania).
    func spareVolumes(in volumes: [VolumeInfo]) -> [VolumeInfo] {
        recoverySources(id: id, volumes: volumes, fromICloud: false)
    }

    /// Jest kopia udziału A w iCloud z co najmniej jednym punktem READY.
    var hasACopy: Bool { (aCopy?.points ?? 0) > 0 }

    var statusLine: String {
        if let e = error { return "Błąd: \(e)" }
        if card == nil { return "Karta robocza niepodłączona" }
        if mounted { return "Zamontowany" }
        return "Gotowy do zamontowania"
    }
}

/// Źródła udziału B do odzyskiwania: karta zapasowa sejfu, a przy A z iCloud
/// (utrata SSD) także karta robocza (S008 K-04).
public func recoverySources(id: String, volumes: [VolumeInfo], fromICloud: Bool) -> [VolumeInfo] {
    volumes.filter { $0.vaults.contains { $0.id == id && ($0.isSpare || fromICloud) } }
}

public func formatBytes(_ n: UInt64) -> String {
    ByteCountFormatter.string(fromByteCount: Int64(clamping: n), countStyle: .file)
}
