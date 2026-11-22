import Foundation

// Wymagania (S011): wynik `xordock doctor --json` i reguły startu aplikacji.

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

    /// Próba montowania potrzebna: macFUSE jest, rozszerzenie niepotwierdzone
    /// dla tej wersji macFUSE (ani próbą, ani udanym montowaniem sejfu).
    public func needsProbe(confirmedFor: String?) -> Bool {
        guard check("macfuse")?.level == "ok", check("extension")?.level == "unknown" else { return false }
        return confirmedFor == nil || confirmedFor != macfuseVersion
    }

    /// Rozszerzenie potwierdzone wcześniej: pokazujemy je jako „ok”.
    public func applyingConfirmation(_ confirmedFor: String?) -> DoctorResult {
        guard let v = confirmedFor, v == macfuseVersion else { return self }
        var r = self
        for i in r.checks.indices where r.checks[i].id == "extension" && r.checks[i].level == "unknown" {
            r.checks[i].level = "ok"
            r.checks[i].detail = "Potwierdzone udanym montowaniem."
        }
        return r
    }

    /// Wersja macFUSE, dla której próba właśnie potwierdziła rozszerzenie.
    public var confirmedVersion: String? {
        check("extension")?.level == "ok" ? macfuseVersion : nil
    }
}

public extension Commands {
    static func doctor(probe: Bool) -> [String] { ["doctor", "--json"] + (probe ? ["--probe-mount"] : []) }
}
