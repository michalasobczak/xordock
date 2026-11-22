import Foundation

public struct CLIError: Error, LocalizedError, Sendable {
    public var message: String
    public var status: Int32
    public var errorDescription: String? { message }
    public init(message: String, status: Int32) {
        self.message = message
        self.status = status
    }
}

/// Uruchamia dołączony `xordock` przez Process z tablicą argumentów (bez powłoki).
public struct CLI: Sendable {
    public let binary: URL

    public init(binary: URL) { self.binary = binary }

    /// W pakiecie .app wyłącznie Contents/Resources/xordock (podpisany razem
    /// z aplikacją; zmienna środowiskowa ani katalog roboczy nie podmienią
    /// silnika). Poza pakietem (`swift run`, testy): XORDOCK_CLI albo bin/xordock.
    public static func locate(bundle: Bundle = .main, environment: [String: String] = ProcessInfo.processInfo.environment,
                              cwd: String = FileManager.default.currentDirectoryPath) -> CLI? {
        var candidates: [URL] = []
        if bundle.bundleURL.pathExtension == "app" {
            if let r = bundle.resourceURL { candidates.append(r.appendingPathComponent("xordock")) }
        } else {
            if let env = environment["XORDOCK_CLI"] { candidates.append(URL(fileURLWithPath: env)) }
            let base = URL(fileURLWithPath: cwd)
            candidates.append(base.appendingPathComponent("../bin/xordock").standardized)
            candidates.append(base.appendingPathComponent("bin/xordock"))
        }
        return candidates.first { FileManager.default.isExecutableFile(atPath: $0.path) }.map(CLI.init)
    }

    /// Uruchamia polecenie i czeka na wynik. Błąd: kod ≠ 0, komunikat ze stderr.
    public func run(_ args: [String]) async throws -> String {
        let bin = binary
        return try await Task.detached {
            let p = Process()
            p.executableURL = bin
            p.arguments = args
            let out = Pipe(), err = Pipe()
            p.standardOutput = out
            p.standardError = err
            p.standardInput = FileHandle.nullDevice
            try p.run()
            async let o = out.fileHandleForReading.readToEnd()
            async let e = err.fileHandleForReading.readToEnd()
            let (od, ed) = (try? await o, try? await e)
            p.waitUntilExit()
            let stdout = String(decoding: (od ?? nil) ?? Data(), as: UTF8.self)
            let stderr = CLI.cleanStderr(String(decoding: (ed ?? nil) ?? Data(), as: UTF8.self))
            if p.terminationStatus != 0 {
                let msg = stderr.isEmpty ? stdout : stderr
                throw CLIError(message: msg.isEmpty ? "xordock zakończył się kodem \(p.terminationStatus)" : msg, status: p.terminationStatus)
            }
            return stdout
        }.value
    }

    public func query<T: Decodable>(_ type: T.Type, _ args: [String]) async throws -> T {
        let out = try await run(args)
        return try jsonDecoder.decode(T.self, from: Data(out.utf8))
    }

    /// Usuwa szum libfuse (R4-04) i prefiks „xordock:” z komunikatu błędu.
    public static func cleanStderr(_ s: String) -> String {
        s.split(separator: "\n", omittingEmptySubsequences: true)
            .map(String.init)
            .filter { !$0.hasPrefix("fuse: forking a threaded process") }
            .map { $0.hasPrefix("xordock: ") ? String($0.dropFirst(9)) : $0 }
            .joined(separator: "\n")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Startuje proces montowania odłączony od aplikacji: wyjście do pliku logu,
    /// nie do potoku, więc zamknięcie aplikacji nie zrywa montowania (S007 §5).
    public func spawnMount(name: String, log: URL, onExit: @escaping @Sendable (Int32) -> Void) throws {
        try FileManager.default.createDirectory(at: log.deletingLastPathComponent(), withIntermediateDirectories: true,
                                                attributes: [.posixPermissions: 0o700])
        if !FileManager.default.fileExists(atPath: log.path) {
            FileManager.default.createFile(atPath: log.path, contents: nil, attributes: [.posixPermissions: 0o600])
        }
        let fh = try FileHandle(forWritingTo: log)
        try fh.seekToEnd()
        let stamp = ISO8601DateFormatter().string(from: Date())
        try fh.write(contentsOf: Data("--- \(stamp) mount\n".utf8))
        let p = Process()
        p.executableURL = binary
        p.arguments = Commands.mount(name: name)
        p.standardOutput = fh
        p.standardError = fh
        p.standardInput = FileHandle.nullDevice
        p.terminationHandler = { proc in onExit(proc.terminationStatus) }
        try p.run()
    }

    public static func logURL(name: String) -> URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Logs/xordock/\(name).log")
    }

    /// Ostatnie linie logu montowania (komunikat błędu po nieudanym montowaniu).
    public static func logTail(_ url: URL, lines: Int = 4) -> String {
        guard let s = try? String(contentsOf: url, encoding: .utf8) else { return "" }
        let ls = s.split(separator: "\n").map(String.init).filter { !$0.hasPrefix("fuse: forking") && !$0.hasPrefix("---") }
        return cleanStderr(ls.suffix(lines).joined(separator: "\n"))
    }
}
