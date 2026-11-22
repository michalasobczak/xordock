import AppKit
import SwiftUI
import XordockCore
import XordockUI

/// Co odzyskujemy: nazwa i identyfikator sejfu, skąd udział A.
struct RecoveryTarget: Hashable {
    var name: String
    var id: String
    var fromICloud: Bool
    var mounted: Bool
    var hasACopy: Bool
}

enum SheetKind: Identifiable {
    case newVault, backup(VaultInfo), recovery(RecoveryTarget)
    var id: String {
        switch self {
        case .newVault: return "new"
        case .backup(let v): return "b-" + v.name
        case .recovery(let r): return "r-" + r.name
        }
    }
}

struct MainView: View {
    @Bindable var model: AppModel
    @State private var sheet: SheetKind?

    var body: some View {
        NavigationSplitView {
            List(selection: $model.selection) {
                Section("Sejfy") {
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
                    Section("Kopie A w iCloud bez SSD") {
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
                    Label("Nowy sejf…", systemImage: "plus")
                }
                .padding(8)
                .disabled(model.busy != nil)
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
                    ContentUnavailableView("Brak sejfów", systemImage: "lock.shield",
                                           description: Text("Załóż pierwszy sejf przyciskiem „Nowy sejf…”."))
                }
            }
        }
        .sheet(item: $sheet) { kind in
            switch kind {
            case .newVault: NewVaultSheet(model: model)
            case .backup(let v): BackupSheet(model: model, vault: v)
            case .recovery(let r): RecoverySheet(model: model, target: r)
            }
        }
        .toolbar {
            ToolbarItem {
                Button { Task { await model.refresh() } } label: { Label("Odśwież", systemImage: "arrow.clockwise") }
            }
        }
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
            Banner(text: "Nie znaleziono programu xordock. Zbuduj aplikację skryptem scripts/build-app.sh.", style: .error)
        }
        if !model.state.fileVault {
            Banner(text: "FileVault wyłączony: aplikacje mogą zostawiać jawne kopie plików na SSD.", style: .warning)
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
                    Button(expanded ? "Ukryj" : "Szczegóły") { expanded.toggle() }.buttonStyle(.link)
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

/// Kopia A w iCloud, dla której na tym Macu nie ma udziału A (S008).
struct OrphanDetail: View {
    let copy: CopyInfo
    @Binding var sheet: SheetKind?

    var body: some View {
        Form {
            Section {
                LabeledContent("Sejf", value: copy.name)
                LabeledContent("Ostatni punkt", value: copy.lastPoint?.formatted(date: .abbreviated, time: .shortened) ?? "brak")
                Text("Na tym Macu nie ma udziału A tego sejfu (np. po wymianie lub utracie SSD). Dane odzyskasz z tej kopii i karty zapasowej albo roboczej.")
                Button("Odzyskiwanie…") {
                    sheet = .recovery(RecoveryTarget(name: copy.name, id: copy.id, fromICloud: true, mounted: false, hasACopy: true))
                }
                .keyboardShortcut(.defaultAction)
            }
        }
        .formStyle(.grouped)
    }
}

/// Wybór nośnika z listy: nazwa, system plików, wolne miejsce, sejfy na nim.
struct VolumePicker: View {
    let title: String
    let volumes: [VolumeInfo]
    @Binding var selection: VolumeInfo?

    var body: some View {
        Picker(title, selection: $selection) {
            Text("— wybierz —").tag(VolumeInfo?.none)
            ForEach(volumes) { v in
                Text(describe(v)).tag(VolumeInfo?.some(v))
            }
        }
    }

    private func describe(_ v: VolumeInfo) -> String {
        var s = "\(v.name) (\(v.fs.uppercased()), wolne \(formatBytes(v.free)))"
        if !v.vaults.isEmpty { s += " · sejfy: " + v.vaults.map(\.name).joined(separator: ", ") }
        return s
    }
}
