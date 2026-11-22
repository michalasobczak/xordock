import AppKit
import Foundation
import Observation
import XordockCore

/// Stan aplikacji. Jedynym źródłem prawdy jest CLI (`vaults`, `volumes`);
/// model niczego nie czyta z udziałów (S007 G5).
let orphanTag = "icloud:"

@MainActor
@Observable
final class AppModel {
    var state = AppState()
    var volumes: [VolumeInfo] = []
    var selection: String?
    var busy: String?          // opis trwającej akcji
    var message: String?       // ostatni wynik akcji: jedno zdanie
    var output: String?        // pełne wyjście CLI ostatniej akcji (pod „Szczegóły”)
    var errorText: String?     // ostatni błąd
    var cliMissing = false

    let cli: CLI?
    private var timer: Timer?
    private var observers: [NSObjectProtocol] = []

    init() {
        cli = CLI.locate()
        cliMissing = cli == nil
        let nc = NSWorkspace.shared.notificationCenter
        for n in [NSWorkspace.didMountNotification, NSWorkspace.didUnmountNotification, NSWorkspace.didRenameVolumeNotification] {
            observers.append(nc.addObserver(forName: n, object: nil, queue: .main) { [weak self] _ in
                Task { @MainActor in await self?.refresh() }
            })
        }
        timer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refresh() }
        }
        Task { await refresh() }
    }

    var selected: VaultInfo? {
        if selection?.hasPrefix(orphanTag) == true { return nil }
        return state.vaults.first { $0.name == selection } ?? state.vaults.first
    }
    var selectedOrphan: CopyInfo? {
        guard let s = selection, s.hasPrefix(orphanTag) else { return nil }
        return state.orphanCopies.first { orphanTag + $0.name == s }
    }

    func refresh() async {
        guard let cli else { return }
        do {
            async let s = cli.query(AppState.self, Commands.vaults())
            async let v = cli.query([VolumeInfo].self, Commands.volumes())
            let (ns, nv) = try await (s, v)
            if ns != state { state = ns }
            if nv != volumes { volumes = nv }
            let valid = state.vaults.contains { $0.name == selection } || state.orphanCopies.contains { orphanTag + $0.name == selection }
            if selection == nil || !valid {
                selection = state.vaults.first?.name ?? state.orphanCopies.first.map { orphanTag + $0.name }
            }
        } catch {
            errorText = "Nie udało się odczytać stanu: \(error.localizedDescription)"
        }
    }

    /// Wykonuje akcję CLI z blokadą przycisków i komunikatem wyniku.
    @discardableResult
    func perform(_ title: String, _ steps: [[String]], success: String = "Gotowe.") async -> Bool {
        guard let cli, busy == nil else { return false }
        busy = title
        errorText = nil
        message = nil
        output = nil
        defer { busy = nil }
        var outs: [String] = []
        for args in steps {
            do {
                let out = try await cli.run(args)
                outs.append(CLI.cleanStderr(out))
            } catch {
                errorText = error.localizedDescription
                await refresh()
                return false
            }
        }
        let full = outs.filter { !$0.isEmpty }.joined(separator: "\n")
        output = full.isEmpty ? nil : full
        message = success
        await refresh()
        return true
    }

    // MARK: akcje

    func mount(_ v: VaultInfo) async {
        guard let cli, busy == nil else { return }
        busy = "Montowanie…"
        errorText = nil
        message = nil
        let log = CLI.logURL(name: v.name)
        let name = v.name
        do {
            try cli.spawnMount(name: name, log: log) { [weak self] status in
                Task { @MainActor in
                    guard let self else { return }
                    if status != 0 {
                        let tail = CLI.logTail(log)
                        self.errorText = "Sejf „\(name)” odmontowany z błędem: \(tail.isEmpty ? "kod \(status)" : tail)"
                    }
                    await self.refresh()
                }
            }
        } catch {
            errorText = error.localizedDescription
            busy = nil
            return
        }
        // Czekamy, aż wolumin się pojawi (do 15 s).
        for _ in 0..<30 {
            try? await Task.sleep(for: .milliseconds(500))
            await refresh()
            if state.vaults.first(where: { $0.name == name })?.mounted == true {
                message = "Sejf „\(name)” otwarty w Finderze."
                busy = nil
                NSWorkspace.shared.open(URL(fileURLWithPath: v.mountpoint))
                return
            }
            if errorText != nil { break }
        }
        busy = nil
        if errorText == nil { errorText = "Montowanie nie powiodło się. Log: \(log.path)" }
    }

    func unmount(_ v: VaultInfo) async -> Bool {
        let ok = await perform("Odmontowywanie…", [Commands.unmount(name: v.name)], success: "Sejf „\(v.name)” odmontowany. Kartę można wyjąć.")
        guard ok else { return false }
        for _ in 0..<20 {
            if state.vaults.first(where: { $0.name == v.name })?.mounted != true { return true }
            try? await Task.sleep(for: .milliseconds(500))
            await refresh()
        }
        errorText = "Wolumin nadal zamontowany. Zamknij pliki otwarte z sejfu i spróbuj ponownie."
        return false
    }

    func create(name: String, card: VolumeInfo) async -> Bool {
        let ok = await perform("Zakładanie sejfu…", [Commands.initVault(name: name, card: card.path)], success: "Sejf „\(name)” założony.")
        if ok { selection = name }
        return ok
    }

    /// Backup teraz: (odmontuj) → migawka → backup (S004 A-11, S007 U-04).
    func backup(_ v: VaultInfo, spare: VolumeInfo) async -> Bool {
        if v.mounted {
            guard await unmount(v) else { return false }
        }
        return await perform("Migawka i backup…", [Commands.snapshot(name: v.name), Commands.backup(name: v.name, spare: spare.path)],
                             success: "Backup gotowy: kopia B na „\(spare.name)”\(state.icloud ? ", kopia A w iCloud" : "").")
    }

    func fsck(_ v: VaultInfo) async {
        await perform("Sprawdzanie spójności…", [Commands.fsck(name: v.name)], success: "Sejf spójny.")
    }

    func points(name: String, spare: VolumeInfo, fromICloud: Bool) async throws -> PointsResult {
        guard let cli else { throw CLIError(message: "brak CLI", status: -1) }
        return try await cli.query(PointsResult.self, Commands.points(name: name, spare: spare.path, fromICloud: fromICloud))
    }

    /// Jednorazowe ostrzeżenie przed pierwszą kopią udziału A do iCloud (S008).
    var adpAccepted: Bool {
        get { UserDefaults.standard.bool(forKey: "adpWarningAccepted") }
        set { UserDefaults.standard.set(newValue, forKey: "adpWarningAccepted") }
    }

    func showInFinder(_ path: String) {
        NSWorkspace.shared.open(URL(fileURLWithPath: path))
    }

    func revealLog(_ v: VaultInfo) {
        NSWorkspace.shared.activateFileViewerSelecting([CLI.logURL(name: v.name)])
    }
}
