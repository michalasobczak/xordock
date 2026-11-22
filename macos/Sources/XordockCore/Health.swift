import Foundation

// Vault dashboard rules (S009): one status with one recommendation and four
// drive tiles. Pure functions, no UI, tested in XordockCoreTests.

public enum Level: Equatable, Sendable {
    case ok, warning, error, neutral
}

/// Main action recommended in a given state.
public enum PrimaryAction: Equatable, Sendable {
    case none, mount, showInFinder, backup, details
}

public struct VaultHealth: Equatable, Sendable {
    public var level: Level
    public var title: String
    public var detail: String
    public var action: PrimaryAction
    public var actionTitle: String?

    /// First matching rule by S009 P-01 priority.
    public static func evaluate(_ v: VaultInfo, icloud: Bool, now: Date = Date()) -> VaultHealth {
        if let e = v.error {
            return VaultHealth(level: .error, title: "Vault problem",
                               detail: String(e.prefix(110)), action: .details, actionTitle: "Details")
        }
        if v.card == nil {
            return VaultHealth(level: .warning, title: "Connect the working card",
                               detail: "The vault can't be opened without the working card.", action: .none, actionTitle: nil)
        }
        if v.lastBackup == nil {
            return VaultHealth(level: .warning, title: "No backup",
                               detail: "Losing the working card today means losing data.", action: .backup, actionTitle: "Back up now")
        }
        if v.backupDue {
            return VaultHealth(level: .warning, title: "Backup older than a week",
                               detail: changes(v) ?? "Make a new backup to the spare card.", action: .backup, actionTitle: "Back up now")
        }
        if icloud && !v.hasACopy {
            return VaultHealth(level: .warning, title: "No iCloud copy of A",
                               detail: "Losing the SSD today means losing data.", action: .backup, actionTitle: "Back up now")
        }
        if v.mounted {
            return VaultHealth(level: .ok, title: "Vault open",
                               detail: "Unmount before removing the card.", action: .showInFinder, actionTitle: "Show in Finder")
        }
        return VaultHealth(level: .ok, title: "All good",
                           detail: changes(v) ?? "Backups are up to date.", action: .mount, actionTitle: "Open vault")
    }

    static func changes(_ v: VaultInfo) -> String? {
        switch v.changesSinceBackup {
        case 0: return nil
        case 1: return "1 change since the last backup."
        default: return "\(v.changesSinceBackup) changes since the last backup."
        }
    }
}

public struct Tile: Equatable, Identifiable, Sendable {
    public enum Kind: String, Sendable { case ssd, card, spare, icloud }
    public var kind: Kind
    public var title: String     // drive
    public var role: String      // what it holds, in one or two words
    public var level: Level
    public var status: String    // short: "connected", "Nov 22, 7:21 PM", "no backup"
    public var note: String?     // e.g. volume name
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

/// Date in English regardless of the system language (the whole UI is in English).
public func shortDate(_ d: Date) -> String {
    d.formatted(.dateTime.day().month(.abbreviated).hour().minute().locale(Locale(identifier: "en_US")))
}

public extension VaultInfo {
    /// Drive tiles (S009 P-02).
    func tiles(icloud: Bool) -> [Tile] {
        let ssd = Tile(kind: .ssd, title: "This Mac", role: "share A", level: error == nil ? .ok : .error,
                       status: error == nil ? "in place" : "error", note: nil)
        let cardTile: Tile = {
            guard let vol = cardVolume else {
                return Tile(kind: .card, title: "Working card", role: "share B", level: .warning, status: "not connected", note: nil)
            }
            return Tile(kind: .card, title: "Working card", role: "share B", level: .ok,
                        status: mounted ? "vault open" : "connected", note: (vol as NSString).lastPathComponent)
        }()
        let spareNote = spares.first.map { "\(($0.volume as NSString).lastPathComponent) connected" }
        let spare: Tile = {
            guard let b = lastBackup else {
                return Tile(kind: .spare, title: "Spare card", role: "copy of B", level: .warning, status: "no backup", note: spareNote)
            }
            return Tile(kind: .spare, title: "Spare card", role: "copy of B", level: backupDue ? .warning : .ok,
                        status: shortDate(b.time), note: spareNote)
        }()
        let cloud: Tile = {
            if !icloud {
                return Tile(kind: .icloud, title: "iCloud Drive", role: "copy of A", level: .neutral, status: "off", note: nil)
            }
            guard let c = aCopy, c.points > 0 else {
                return Tile(kind: .icloud, title: "iCloud Drive", role: "copy of A", level: .warning, status: "no copy", note: nil)
            }
            return Tile(kind: .icloud, title: "iCloud Drive", role: "copy of A", level: backupDue ? .warning : .ok,
                        status: c.lastPoint.map(shortDate) ?? "present", note: nil)
        }()
        return [ssd, cardTile, spare, cloud]
    }

    var health: Level { VaultHealth.evaluate(self, icloud: true).level }
}
