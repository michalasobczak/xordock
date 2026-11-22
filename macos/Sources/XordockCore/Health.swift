import Foundation

// Reguły pulpitu sejfu (S009): jeden status z jednym zaleceniem i cztery
// kafelki nośników. Czyste funkcje, bez UI, testowane w XordockCoreTests.

public enum Level: Equatable, Sendable {
    case ok, warning, error, neutral
}

/// Główna akcja zalecana w danym stanie.
public enum PrimaryAction: Equatable, Sendable {
    case none, mount, showInFinder, backup, details
}

public struct VaultHealth: Equatable, Sendable {
    public var level: Level
    public var title: String
    public var detail: String
    public var action: PrimaryAction
    public var actionTitle: String?

    /// Pierwsza pasująca reguła wg priorytetu S009 P-01.
    public static func evaluate(_ v: VaultInfo, icloud: Bool, now: Date = Date()) -> VaultHealth {
        if let e = v.error {
            return VaultHealth(level: .error, title: "Problem z sejfem",
                               detail: String(e.prefix(110)), action: .details, actionTitle: "Szczegóły")
        }
        if v.card == nil {
            return VaultHealth(level: .warning, title: "Podłącz kartę roboczą",
                               detail: "Bez karty roboczej sejfu nie da się otworzyć.", action: .none, actionTitle: nil)
        }
        if v.lastBackup == nil {
            return VaultHealth(level: .warning, title: "Brak backupu",
                               detail: "Utrata karty roboczej oznacza dziś utratę danych.", action: .backup, actionTitle: "Zrób backup")
        }
        if v.backupDue {
            return VaultHealth(level: .warning, title: "Backup starszy niż tydzień",
                               detail: changes(v) ?? "Zrób nowy backup na kartę zapasową.", action: .backup, actionTitle: "Zrób backup")
        }
        if icloud && !v.hasACopy {
            return VaultHealth(level: .warning, title: "Brak kopii A w iCloud",
                               detail: "Utrata SSD oznacza dziś utratę danych.", action: .backup, actionTitle: "Zrób backup")
        }
        if v.mounted {
            return VaultHealth(level: .ok, title: "Sejf otwarty",
                               detail: "Odmontuj przed wyjęciem karty.", action: .showInFinder, actionTitle: "Pokaż w Finderze")
        }
        return VaultHealth(level: .ok, title: "Wszystko w porządku",
                           detail: changes(v) ?? "Kopie są aktualne.", action: .mount, actionTitle: "Otwórz sejf")
    }

    static func changes(_ v: VaultInfo) -> String? {
        switch v.changesSinceBackup {
        case 0: return nil
        case 1: return "1 zmiana od ostatniego backupu."
        case 2...4: return "\(v.changesSinceBackup) zmiany od ostatniego backupu."
        default: return "\(v.changesSinceBackup) zmian od ostatniego backupu."
        }
    }
}

public struct Tile: Equatable, Identifiable, Sendable {
    public enum Kind: String, Sendable { case ssd, card, spare, icloud }
    public var kind: Kind
    public var title: String     // nośnik
    public var role: String      // co na nim leży, jednym-dwoma słowami
    public var level: Level
    public var status: String    // krótko: „podłączona”, „22 lis, 19:21”, „brak”
    public var note: String?     // np. nazwa woluminu
    public var id: String { kind.rawValue }
    public var symbol: String {
        switch kind {
        case .ssd: return "laptopcomputer"
        case .card: return "sdcard"
        case .spare: return "externaldrive"
        case .icloud: return "icloud"
        }
    }
}

/// Data po polsku niezależnie od języka systemu (cały interfejs jest po polsku).
public func shortDate(_ d: Date) -> String {
    d.formatted(.dateTime.day().month(.abbreviated).hour().minute().locale(Locale(identifier: "pl_PL")))
}

public extension VaultInfo {
    /// Kafelki nośników (S009 P-02).
    func tiles(icloud: Bool) -> [Tile] {
        let ssd = Tile(kind: .ssd, title: "Ten Mac", role: "udział A", level: error == nil ? .ok : .error,
                       status: error == nil ? "na miejscu" : "błąd", note: nil)
        let cardTile: Tile = {
            guard let vol = cardVolume else {
                return Tile(kind: .card, title: "Karta robocza", role: "udział B", level: .warning, status: "niepodłączona", note: nil)
            }
            return Tile(kind: .card, title: "Karta robocza", role: "udział B", level: .ok,
                        status: mounted ? "sejf otwarty" : "podłączona", note: (vol as NSString).lastPathComponent)
        }()
        let spareNote = spares.first.map { "\(($0.volume as NSString).lastPathComponent) podłączona" }
        let spare: Tile = {
            guard let b = lastBackup else {
                return Tile(kind: .spare, title: "Karta zapasowa", role: "kopia B", level: .warning, status: "brak backupu", note: spareNote)
            }
            return Tile(kind: .spare, title: "Karta zapasowa", role: "kopia B", level: backupDue ? .warning : .ok,
                        status: shortDate(b.time), note: spareNote)
        }()
        let cloud: Tile = {
            if !icloud {
                return Tile(kind: .icloud, title: "iCloud Drive", role: "kopia A", level: .neutral, status: "wyłączony", note: nil)
            }
            guard let c = aCopy, c.points > 0 else {
                return Tile(kind: .icloud, title: "iCloud Drive", role: "kopia A", level: .warning, status: "brak kopii", note: nil)
            }
            return Tile(kind: .icloud, title: "iCloud Drive", role: "kopia A", level: backupDue ? .warning : .ok,
                        status: c.lastPoint.map(shortDate) ?? "jest", note: nil)
        }()
        return [ssd, cardTile, spare, cloud]
    }

    var health: Level { VaultHealth.evaluate(self, icloud: true).level }
}
