import AppKit
import SwiftUI
import XordockCore
import XordockUI

/// What we recover: vault name and ID, where share A comes from.
struct RecoveryTarget: Hashable {
    var name: String
    var id: String
    var fromICloud: Bool
    var mounted: Bool
    var hasACopy: Bool
}

enum SheetKind: Identifiable {
    case newVault, backup(VaultInfo), recovery(RecoveryTarget), startPrompt, intro, requirements
    var id: String {
        switch self {
        case .requirements: return "req"
        case .startPrompt: return "start"
        case .intro: return "intro"
        case .newVault: return "new"
        case .backup(let v): return "b-" + v.name
        case .recovery(let r): return "r-" + r.name
        }
    }
}

struct MainView: View {
    @Bindable var model: AppModel
    @State private var sheet: SheetKind?
    @AppStorage(AppModel.askKey) private var askAtLaunch = true

    var body: some View {
        NavigationSplitView {
            List(selection: $model.selection) {
                Section("Vaults") {
                    ForEach(model.state.vaults) { v in
                        HStack {
                            Image(systemName: v.mounted ? "lock.open.fill" : "lock.fill").foregroundStyle(.secondary)
                            Text(v.name)
                            Spacer()
                            HealthDot(level: VaultHealth.evaluate(v, icloud: model.state.icloud).level)
                        }
                        .tag(v.name)
                    }
                }
                if !model.state.orphanCopies.isEmpty {
                    Section("Share A copies in iCloud without SSD") {
                        ForEach(model.state.orphanCopies) { c in
                            Label(c.name, systemImage: "icloud").tag(orphanTag + c.name)
                        }
                    }
                }
            }
            .safeAreaInset(edge: .bottom) {
                Button {
                    sheet = .newVault
                } label: {
                    Label("New vault…", systemImage: "plus")
                }
                .padding(8)
                .disabled(model.busy != nil)
                .tourAnchor(.newVault, edge: .trailing)
            }
            .navigationSplitViewColumnWidth(min: 200, ideal: 240)
        } detail: {
            VStack(spacing: 0) {
                banners
                if let c = model.selectedOrphan {
                    OrphanDetail(copy: c, sheet: $sheet)
                } else if let v = model.selected {
                    Dashboard(vault: v, icloud: model.state.icloud, masks: model.state.masks, busy: model.busy != nil,
                              actions: actions(for: v))
                } else {
                    ContentUnavailableView("No vaults", systemImage: "lock.shield",
                                           description: Text("Create your first vault with the \"New vault…\" button."))
                }
                Spacer(minLength: 0)
                TourBar()
            }
        }
        .environment(model.tour)
        .onAppear(perform: checkPending)
        .onChange(of: model.pendingStartPrompt) { checkPending() }
        .onChange(of: model.pendingIntro) { checkPending() }
        .onChange(of: model.pendingRequirements) { checkPending() }
        .sheet(item: $sheet, onDismiss: checkPending) { kind in
            switch kind {
            case .requirements:
                RequirementsView(result: model.requirements, checking: model.checkingRequirements,
                                 onRecheck: { Task { await model.checkRequirements(forceProbe: true) } },
                                 onClose: { sheet = nil })
            case .startPrompt:
                StartPrompt(askAtLaunch: $askAtLaunch, onStart: { sheet = .intro }, onLater: { sheet = nil })
            case .intro:
                IntroView(onFinish: startTour, onSkip: startTour)
            case .newVault: NewVaultSheet(model: model)
            case .backup(let v): BackupSheet(model: model, vault: v)
            case .recovery(let r): RecoverySheet(model: model, target: r)
            }
        }
        .toolbar {
            ToolbarItem {
                Button { Task { await model.refresh() } } label: { Label("Refresh", systemImage: "arrow.clockwise") }
            }
        }
    }

    private func checkPending() {
        guard sheet == nil else { return }
        if model.pendingRequirements {
            // Missing requirements take precedence over the tutorial prompt (S011).
            model.pendingRequirements = false
            sheet = .requirements
        } else if model.pendingStartPrompt {
            model.pendingStartPrompt = false
            sheet = .startPrompt
        } else if model.pendingIntro {
            model.pendingIntro = false
            sheet = .intro
        }
    }

    private func startTour() {
        sheet = nil
        // Show the tip after the sheet closes so the popover has an anchor.
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.4) { model.tour.start() }
    }

    private func actions(for v: VaultInfo) -> DashboardActions {
        var a = DashboardActions()
        a.primary = { action in
            switch action {
            case .mount: Task { await model.mount(v) }
            case .showInFinder: model.showInFinder(v.mountpoint)
            case .backup: sheet = .backup(v)
            case .details: model.errorText = v.error
            case .none: break
            }
        }
        a.mount = { Task { await model.mount(v) } }
        a.unmount = { Task { _ = await model.unmount(v) } }
        a.backup = { sheet = .backup(v) }
        a.recovery = {
            sheet = .recovery(RecoveryTarget(name: v.name, id: v.id, fromICloud: false, mounted: v.mounted, hasACopy: v.hasACopy))
        }
        a.fsck = { Task { await model.fsck(v) } }
        a.showLog = { model.revealLog(v) }
        return a
    }

    @ViewBuilder private var banners: some View {
        if model.cliMissing {
            Banner(text: "The xordock program was not found. Build the app with scripts/build-app.sh.", style: .error)
        }
        if !model.state.fileVault {
            Banner(text: "FileVault is off: apps may leave plaintext copies of files on the SSD.", style: .warning)
        }
        if let b = model.busy {
            HStack { ProgressView().controlSize(.small); Text(b) }.padding(8)
        }
        if let e = model.errorText {
            Banner(text: e, style: .error) { model.errorText = nil }
        }
        if let m = model.message, !m.isEmpty {
            Banner(text: m, style: .info, details: model.output) { model.message = nil; model.output = nil }
        }
    }
}

struct Banner: View {
    enum Style { case info, warning, error }
    let text: String
    let style: Style
    var details: String? = nil
    var onClose: (() -> Void)?
    @State private var expanded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .top) {
                Image(systemName: style == .info ? "checkmark.circle" : "exclamationmark.triangle")
                Text(text).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
                if details != nil {
                    Button(expanded ? "Hide" : "Details") { expanded.toggle() }.buttonStyle(.link)
                }
                if let onClose {
                    Button { onClose() } label: { Image(systemName: "xmark") }.buttonStyle(.borderless)
                }
            }
            if expanded, let d = details {
                Text(d).font(.caption.monospaced()).textSelection(.enabled).foregroundStyle(.secondary)
            }
        }
        .padding(10)
        .background(color.opacity(0.15))
    }

    private var color: Color {
        switch style {
        case .info: return .green
        case .warning: return .orange
        case .error: return .red
        }
    }
}

/// Copy of A in iCloud for which this Mac has no share A (S008).
struct OrphanDetail: View {
    let copy: CopyInfo
    @Binding var sheet: SheetKind?

    var body: some View {
        Form {
            Section {
                LabeledContent("Vault", value: copy.name)
                LabeledContent("Last point", value: copy.lastPoint?.formatted(date: .abbreviated, time: .shortened) ?? "none")
                Text("This Mac doesn't have share A of this vault (e.g. after replacing or losing the SSD). You can recover the data from this copy and the spare or working card.")
                Button("Recovery…") {
                    sheet = .recovery(RecoveryTarget(name: copy.name, id: copy.id, fromICloud: true, mounted: false, hasACopy: true))
                }
                .keyboardShortcut(.defaultAction)
            }
        }
        .formStyle(.grouped)
    }
}

/// Drive picker: name, file system, free space, vaults on it.
struct VolumePicker: View {
    let title: String
    let volumes: [VolumeInfo]
    @Binding var selection: VolumeInfo?

    var body: some View {
        Picker(title, selection: $selection) {
            Text("— choose —").tag(VolumeInfo?.none)
            ForEach(volumes) { v in
                Text(describe(v)).tag(VolumeInfo?.some(v))
            }
        }
    }

    private func describe(_ v: VolumeInfo) -> String {
        var s = "\(v.name) (\(v.fs.uppercased()), free \(formatBytes(v.free)))"
        if !v.vaults.isEmpty { s += " · vaults: " + v.vaults.map(\.name).joined(separator: ", ") }
        return s
    }
}
