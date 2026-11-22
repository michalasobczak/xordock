import AppKit
import Foundation
import Observation
import XordockCore
import XordockUI

/// App state. The only source of truth is the CLI (`vaults`, `volumes`);
/// the model reads nothing from the shares (S007 G5).
let orphanTag = "icloud:"

@MainActor
@Observable
final class AppModel {
    var state = AppState()
    var volumes: [VolumeInfo] = []
    var selection: String?
    var busy: String?          // description of the running action
    var message: String?       // result of the last action: one sentence
    var output: String?        // full CLI output of the last action (under "Details")
    var errorText: String?     // last error
    var cliMissing = false

    let cli: CLI?
    let tour = TourController()
    /// The window should show the tutorial prompt (launch) or go straight to the introduction.
    var pendingStartPrompt = false
    var pendingIntro = false
    /// Requirements (S011): result of the last check and the sheet to show.
    var requirements: DoctorResult?
    var checkingRequirements = false
    var pendingRequirements = false
    static let confirmedKey = "fskitConfirmedForMacFUSE"
    private var timer: Timer?
    private var observers: [NSObjectProtocol] = []

    init() {
        cli = CLI.locate()
        cliMissing = cli == nil
        UserDefaults.standard.register(defaults: [Self.askKey: true])
        pendingStartPrompt = UserDefaults.standard.bool(forKey: Self.askKey)
        let nc = NSWorkspace.shared.notificationCenter
        for n in [NSWorkspace.didMountNotification, NSWorkspace.didUnmountNotification, NSWorkspace.didRenameVolumeNotification] {
            observers.append(nc.addObserver(forName: n, object: nil, queue: .main) { [weak self] _ in
                Task { @MainActor in await self?.refresh() }
            })
        }
        timer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refresh() }
        }
        tour.vault = { [weak self] in self?.selected }
    }

    var selected: VaultInfo? {
        if selection?.hasPrefix(orphanTag) == true { return nil }
        return state.vaults.first { $0.name == selection } ?? state.vaults.first
    }
    var selectedOrphan: CopyInfo? {
        guard let s = selection, s.hasPrefix(orphanTag) else { return nil }
        return state.orphanCopies.first { orphanTag + $0.name == s }
    }

    static let askKey = "askTutorialAtLaunch"

    /// App launch: requirements, then the vaults' state. Returns true when the window
    /// should open (missing requirements or the tutorial prompt).
    func startup() async -> Bool {
        await checkRequirements(forceProbe: false)
        await refresh()
        return pendingRequirements || pendingStartPrompt
    }

    /// `doctor`; a test mount when the extension is unconfirmed for this
    /// macFUSE version or on request (S011 W-04).
    func checkRequirements(forceProbe: Bool) async {
        guard let cli, !checkingRequirements else { return }
        checkingRequirements = true
        defer { checkingRequirements = false }
        let confirmed = UserDefaults.standard.string(forKey: Self.confirmedKey)
        do {
            var r = try await cli.query(DoctorResult.self, Commands.doctor(probe: false))
            if forceProbe || r.needsProbe(confirmedFor: confirmed) {
                r = try await cli.query(DoctorResult.self, Commands.doctor(probe: true))
                if let v = r.confirmedVersion {
                    UserDefaults.standard.set(v, forKey: Self.confirmedKey)
                } else if r.hasErrors {
                    UserDefaults.standard.removeObject(forKey: Self.confirmedKey)
                }
            }
            requirements = r.applyingConfirmation(UserDefaults.standard.string(forKey: Self.confirmedKey))
            if requirements?.hasErrors == true { pendingRequirements = true }
        } catch {
            requirements = DoctorResult(checks: [RequirementCheck(id: "cli", title: "xordock engine", level: "error",
                                                                  detail: error.localizedDescription, fixLabel: nil, fixURL: nil)])
            pendingRequirements = true
        }
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
            tour.update()
        } catch {
            errorText = "Couldn't read the state: \(error.localizedDescription)"
        }
    }

    /// Runs a CLI action with buttons locked and a result message.
    @discardableResult
    func perform(_ title: String, _ steps: [[String]], success: String = "Done.") async -> Bool {
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

    // MARK: actions

    func mount(_ v: VaultInfo) async {
        guard let cli, busy == nil else { return }
        busy = "Mounting…"
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
                        self.errorText = "Vault \"\(name)\" unmounted with an error: \(tail.isEmpty ? "code \(status)" : tail)"
                    }
                    await self.refresh()
                }
            }
        } catch {
            errorText = error.localizedDescription
            busy = nil
            return
        }
        // Wait for the volume to appear (up to 15 s).
        for _ in 0..<30 {
            try? await Task.sleep(for: .milliseconds(500))
            await refresh()
            if state.vaults.first(where: { $0.name == name })?.mounted == true {
                message = "Vault \"\(name)\" opened in Finder."
                // A successful mount confirms the extension is enabled (S011).
                if let v = requirements?.macfuseVersion {
                    UserDefaults.standard.set(v, forKey: Self.confirmedKey)
                }
                busy = nil
                NSWorkspace.shared.open(URL(fileURLWithPath: v.mountpoint))
                return
            }
            if errorText != nil { break }
        }
        busy = nil
        if errorText == nil { errorText = "Mounting failed. Log: \(log.path)" }
    }

    func unmount(_ v: VaultInfo) async -> Bool {
        let ok = await perform("Unmounting…", [Commands.unmount(name: v.name)], success: "Vault \"\(v.name)\" unmounted. You can remove the card.")
        guard ok else { return false }
        for _ in 0..<20 {
            if state.vaults.first(where: { $0.name == v.name })?.mounted != true { return true }
            try? await Task.sleep(for: .milliseconds(500))
            await refresh()
        }
        errorText = "The volume is still mounted. Close files opened from the vault and try again."
        return false
    }

    func create(name: String, card: VolumeInfo) async -> Bool {
        let ok = await perform("Creating vault…", [Commands.initVault(name: name, card: card.path)], success: "Vault \"\(name)\" created.")
        if ok { selection = name }
        return ok
    }

    /// Back up now: (unmount) → snapshot → backup (S004 A-11, S007 U-04).
    func backup(_ v: VaultInfo, spare: VolumeInfo) async -> Bool {
        if v.mounted {
            guard await unmount(v) else { return false }
        }
        return await perform("Snapshot and backup…", [Commands.snapshot(name: v.name), Commands.backup(name: v.name, spare: spare.path)],
                             success: "Backup done: copy of B on \"\(spare.name)\"\(state.icloud ? ", copy of A in iCloud" : "").")
    }

    func fsck(_ v: VaultInfo) async {
        await perform("Checking consistency…", [Commands.fsck(name: v.name)], success: "Vault is consistent.")
    }

    func points(name: String, spare: VolumeInfo, fromICloud: Bool) async throws -> PointsResult {
        guard let cli else { throw CLIError(message: "CLI missing", status: -1) }
        return try await cli.query(PointsResult.self, Commands.points(name: name, spare: spare.path, fromICloud: fromICloud))
    }

    /// One-time warning before the first copy of share A to iCloud (S008).
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
