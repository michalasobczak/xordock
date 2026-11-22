import Foundation

// Requirements (S011): `xordock doctor --json` result and app launch rules.

public struct RequirementCheck: Decodable, Equatable, Identifiable, Sendable {
    public var id: String
    public var title: String
    public var level: String // ok | warning | error | unknown
    public var detail: String
    public var fixLabel: String?
    public var fixURL: String?

    public init(id: String, title: String, level: String, detail: String, fixLabel: String? = nil, fixURL: String? = nil) {
        self.id = id
        self.title = title
        self.level = level
        self.detail = detail
        self.fixLabel = fixLabel
        self.fixURL = fixURL
    }

    public var levelValue: Level {
        switch level {
        case "ok": return .ok
        case "warning": return .warning
        case "error": return .error
        default: return .neutral
        }
    }
}

public struct DoctorResult: Decodable, Equatable, Sendable {
    public var macfuseVersion: String?
    public var checks: [RequirementCheck]

    public init(macfuseVersion: String? = nil, checks: [RequirementCheck] = []) {
        self.macfuseVersion = macfuseVersion
        self.checks = checks
    }

    public var hasErrors: Bool { checks.contains { $0.level == "error" } }

    func check(_ id: String) -> RequirementCheck? { checks.first { $0.id == id } }

    /// Test mount needed: macFUSE is present, the extension is unconfirmed
    /// for this macFUSE version (neither by a test mount nor by a successful vault mount).
    public func needsProbe(confirmedFor: String?) -> Bool {
        guard check("macfuse")?.level == "ok", check("extension")?.level == "unknown" else { return false }
        return confirmedFor == nil || confirmedFor != macfuseVersion
    }

    /// Extension confirmed earlier: we show it as "ok".
    public func applyingConfirmation(_ confirmedFor: String?) -> DoctorResult {
        guard let v = confirmedFor, v == macfuseVersion else { return self }
        var r = self
        for i in r.checks.indices where r.checks[i].id == "extension" && r.checks[i].level == "unknown" {
            r.checks[i].level = "ok"
            r.checks[i].detail = "Confirmed by a successful mount."
        }
        return r
    }

    /// macFUSE version for which the test just confirmed the extension.
    public var confirmedVersion: String? {
        check("extension")?.level == "ok" ? macfuseVersion : nil
    }
}

public extension Commands {
    static func doctor(probe: Bool) -> [String] { ["doctor", "--json"] + (probe ? ["--probe-mount"] : []) }
}
