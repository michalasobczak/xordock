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

/// Runs the bundled `xordock` via Process with an argument array (no shell).
public struct CLI: Sendable {
    public let binary: URL

    public init(binary: URL) { self.binary = binary }

    /// Inside an .app bundle only Contents/Resources/xordock (signed together
    /// with the app; neither an environment variable nor the working directory can
    /// swap the engine). Outside a bundle (`swift run`, tests): XORDOCK_CLI or bin/xordock.
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

    /// Runs a command and waits for the result. Error: exit code ≠ 0, message from stderr.
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
                throw CLIError(message: msg.isEmpty ? "xordock exited with code \(p.terminationStatus)" : msg, status: p.terminationStatus)
            }
            return stdout
        }.value
    }

    public func query<T: Decodable>(_ type: T.Type, _ args: [String]) async throws -> T {
        let out = try await run(args)
        return try jsonDecoder.decode(T.self, from: Data(out.utf8))
    }

    /// Removes libfuse noise (R4-04) and the "xordock:" prefix from the error message.
    public static func cleanStderr(_ s: String) -> String {
        s.split(separator: "\n", omittingEmptySubsequences: true)
            .map(String.init)
            .filter { !$0.hasPrefix("fuse: forking a threaded process") }
            .map { $0.hasPrefix("xordock: ") ? String($0.dropFirst(9)) : $0 }
            .joined(separator: "\n")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Starts the mount process detached from the app: output goes to a log file,
    /// not a pipe, so quitting the app doesn't break the mount (S007 §5).
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

    /// Last lines of the mount log (error message after a failed mount).
    public static func logTail(_ url: URL, lines: Int = 4) -> String {
        guard let s = try? String(contentsOf: url, encoding: .utf8) else { return "" }
        // Only the latest mount session: lines after the last "--- <time> mount" marker.
        var ls = s.split(separator: "\n").map(String.init)
        if let i = ls.lastIndex(where: { $0.hasPrefix("---") }) { ls = Array(ls[(i + 1)...]) }
        ls = ls.filter { !$0.hasPrefix("fuse: forking") }
        return cleanStderr(ls.suffix(lines).joined(separator: "\n"))
    }
}
