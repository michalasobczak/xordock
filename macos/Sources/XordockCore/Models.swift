import Foundation

// Model of CLI query results (`xordock vaults|volumes|points`, S007 §5).
// Unknown fields are ignored; missing optional fields don't break decoding.

public struct AppState: Decodable, Equatable, Sendable {
    public var fileVault: Bool
    public var masks: String
    public var icloud: Bool
    public var vaults: [VaultInfo]
    public var orphanCopies: [CopyInfo]

    public init(fileVault: Bool = true, masks: String = "", icloud: Bool = true, vaults: [VaultInfo] = [], orphanCopies: [CopyInfo] = []) {
        self.fileVault = fileVault
        self.masks = masks
        self.icloud = icloud
        self.vaults = vaults
        self.orphanCopies = orphanCopies
    }

    enum CodingKeys: String, CodingKey { case fileVault, masks, icloud, vaults, orphanCopies }

    public init(from d: Decoder) throws {
        let c = try d.container(keyedBy: CodingKeys.self)
        fileVault = try c.decodeIfPresent(Bool.self, forKey: .fileVault) ?? true
        masks = try c.decodeIfPresent(String.self, forKey: .masks) ?? ""
        icloud = try c.decodeIfPresent(Bool.self, forKey: .icloud) ?? false
        vaults = try c.decodeIfPresent([VaultInfo].self, forKey: .vaults) ?? []
        orphanCopies = try c.decodeIfPresent([CopyInfo].self, forKey: .orphanCopies) ?? []
    }
}

/// Copy of share A in iCloud Drive (S008).
public struct CopyInfo: Decodable, Equatable, Hashable, Identifiable, Sendable {
    public var name: String
    public var id: String
    public var path: String
    public var points: Int
    public var lastPoint: Date?
}

public struct SpareInfo: Decodable, Equatable, Hashable, Sendable {
    public var root: String
    public var volume: String
    public var media: String
}

public struct BackupInfo: Decodable, Equatable, Sendable {
    public var time: Date
    public var point: String
    public var snapshot: String
    public var seq: UInt64
    public var media: String
}

public struct VaultInfo: Decodable, Equatable, Identifiable, Sendable {
    public var name: String
    public var id: String
    public var a: String
    public var card: String?
    public var spares: [SpareInfo]
    public var mounted: Bool
    public var mountpoint: String
    public var seq: UInt64
    public var snapshots: Int
    public var lastBackup: BackupInfo?
    public var changesSinceBackup: Int
    public var backupDue: Bool
    public var aCopy: CopyInfo?
    public var error: String?

    enum CodingKeys: String, CodingKey {
        case name, id, a, card, spares, mounted, mountpoint, seq, snapshots, lastBackup, changesSinceBackup, backupDue, aCopy, error
    }

    public init(from d: Decoder) throws {
        let c = try d.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? ""
        a = try c.decodeIfPresent(String.self, forKey: .a) ?? ""
        card = try c.decodeIfPresent(String.self, forKey: .card)
        spares = try c.decodeIfPresent([SpareInfo].self, forKey: .spares) ?? []
        mounted = try c.decodeIfPresent(Bool.self, forKey: .mounted) ?? false
        mountpoint = try c.decodeIfPresent(String.self, forKey: .mountpoint) ?? "/Volumes/xordock-\(name)"
        seq = try c.decodeIfPresent(UInt64.self, forKey: .seq) ?? 0
        snapshots = try c.decodeIfPresent(Int.self, forKey: .snapshots) ?? 0
        lastBackup = try c.decodeIfPresent(BackupInfo.self, forKey: .lastBackup)
        changesSinceBackup = try c.decodeIfPresent(Int.self, forKey: .changesSinceBackup) ?? 0
        backupDue = try c.decodeIfPresent(Bool.self, forKey: .backupDue) ?? true
        aCopy = try c.decodeIfPresent(CopyInfo.self, forKey: .aCopy)
        error = try c.decodeIfPresent(String.self, forKey: .error)
    }

    /// Working card volume, e.g. /Volumes/xor_primary.
    public var cardVolume: String? { card.map { volumeOf($0) } }
}

public struct CardRef: Decodable, Equatable, Hashable, Sendable {
    public var name: String
    public var id: String
    public var role: String
    public var root: String
    public var isSpare: Bool { role == "backup" }
}

public struct VolumeInfo: Decodable, Equatable, Hashable, Identifiable, Sendable {
    public var path: String
    public var name: String
    public var fs: String
    public var free: UInt64
    public var total: UInt64
    public var readOnly: Bool
    public var systemDisk: Bool
    public var vaults: [CardRef]
    public var id: String { path }

    enum CodingKeys: String, CodingKey { case path, name, fs, free, total, readOnly, systemDisk, vaults }

    public init(from d: Decoder) throws {
        let c = try d.container(keyedBy: CodingKeys.self)
        path = try c.decode(String.self, forKey: .path)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? (path as NSString).lastPathComponent
        fs = try c.decodeIfPresent(String.self, forKey: .fs) ?? ""
        free = try c.decodeIfPresent(UInt64.self, forKey: .free) ?? 0
        total = try c.decodeIfPresent(UInt64.self, forKey: .total) ?? 0
        readOnly = try c.decodeIfPresent(Bool.self, forKey: .readOnly) ?? false
        systemDisk = try c.decodeIfPresent(Bool.self, forKey: .systemDisk) ?? false
        vaults = try c.decodeIfPresent([CardRef].self, forKey: .vaults) ?? []
    }

    /// The drive can serve as a card (working or spare): writable and not a system disk.
    public var usableAsCard: Bool { !readOnly && !systemDisk }
}

public struct PointInfo: Decodable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var seq: UInt64
    public var snapshot: String
    public var created: Date
    public var objects: Int
    public var bytes: Int64
}

public struct PointsResult: Decodable, Equatable, Sendable {
    public var points: [PointInfo]
    public var rejected: [String]
}

public func volumeOf(_ cardRoot: String) -> String {
    ((cardRoot as NSString).deletingLastPathComponent as NSString).deletingLastPathComponent
}

public let jsonDecoder: JSONDecoder = {
    let d = JSONDecoder()
    d.dateDecodingStrategy = .iso8601
    return d
}()
