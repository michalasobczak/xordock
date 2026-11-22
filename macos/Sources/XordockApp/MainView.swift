import AppKit
import SwiftUI
import XordockCore

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
                        Label {
                            VStack(alignment: .leading) {
                                Text(v.name)
                                Text(v.statusLine).font(.caption).foregroundStyle(.secondary)
                            }
                        } icon: {
                            Image(systemName: v.mounted ? "lock.open.fill" : (v.card == nil ? "externaldrive.badge.xmark" : "lock.fill"))
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
                    VaultDetail(model: model, vault: v, sheet: $sheet)
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

    @ViewBuilder private var banners: some View {
        if model.cliMissing {
            Banner(text: "Nie znaleziono programu xordock. Zbuduj aplikację skryptem scripts/build-app.sh.", style: .error)
        }
        if !model.state.fileVault {
            Banner(text: "FileVault jest wyłączony. Aplikacje mogą zostawiać jawne kopie plików na SSD. Włącz FileVault w Ustawieniach systemowych.", style: .warning)
        }
        if let b = model.busy {
            HStack { ProgressView().controlSize(.small); Text(b) }.padding(8)
        }
        if let e = model.errorText {
            Banner(text: e, style: .error) { model.errorText = nil }
        }
        if let m = model.message, !m.isEmpty {
            Banner(text: m, style: .info) { model.message = nil }
        }
    }
}

struct Banner: View {
    enum Style { case info, warning, error }
    let text: String
    let style: Style
    var onClose: (() -> Void)?

    var body: some View {
        HStack(alignment: .top) {
            Image(systemName: style == .info ? "checkmark.circle" : "exclamationmark.triangle")
            Text(text).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
            if let onClose {
                Button { onClose() } label: { Image(systemName: "xmark") }.buttonStyle(.borderless)
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

struct VaultDetail: View {
    let model: AppModel
    let vault: VaultInfo
    @Binding var sheet: SheetKind?

    var body: some View {
        Form {
            Section {
                LabeledContent("Stan", value: vault.statusLine)
                LabeledContent("Karta robocza (B)", value: vault.cardVolume ?? "niepodłączona")
                LabeledContent("Udział A (SSD)") {
                    Text(vault.a).textSelection(.enabled).font(.callout)
                }
                if vault.mounted {
                    LabeledContent("Wolumin", value: vault.mountpoint)
                }
            }
            Section("Backup") {
                if let b = vault.lastBackup {
                    LabeledContent("Ostatni punkt READY", value: b.time.formatted(date: .abbreviated, time: .shortened))
                    LabeledContent("Zmian od backupu", value: "\(vault.changesSinceBackup)")
                } else if vault.card != nil, vault.error == nil {
                    Text("Brak potwierdzonego punktu odzyskiwania. Utrata karty roboczej oznacza utratę danych.")
                        .foregroundStyle(.orange)
                }
                if vault.card != nil, vault.backupDue, vault.lastBackup != nil {
                    Text("Ostatni backup jest starszy niż 7 dni.").foregroundStyle(.orange)
                }
                if let c = vault.aCopy, c.points > 0 {
                    LabeledContent("Kopia udziału A (iCloud)", value: c.lastPoint?.formatted(date: .abbreviated, time: .shortened) ?? "jest")
                } else if vault.lastBackup != nil {
                    Text(model.state.icloud
                         ? "Brak kopii udziału A w iCloud. Utrata SSD = utrata danych. Zrób backup, żeby powstała."
                         : "iCloud Drive niedostępny: kopia udziału A nie powstaje. Utrata SSD = utrata danych.")
                        .foregroundStyle(.orange)
                }
                LabeledContent("Migawki", value: "\(vault.snapshots)")
                LabeledContent("Karty zapasowe", value: vault.spares.isEmpty ? "brak podłączonych" : vault.spares.map(\.volume).joined(separator: ", "))
            }
            Section {
                HStack {
                    if vault.mounted {
                        Button("Pokaż w Finderze") { model.showInFinder(vault.mountpoint) }
                        Button("Odmontuj") { Task { _ = await model.unmount(vault) } }
                    } else {
                        Button("Zamontuj") { Task { await model.mount(vault) } }
                            .disabled(vault.card == nil || vault.error != nil)
                            .keyboardShortcut(.defaultAction)
                    }
                    Button("Backup teraz…") { sheet = .backup(vault) }
                        .disabled(vault.card == nil)
                    Button("Odzyskiwanie…") {
                        sheet = .recovery(RecoveryTarget(name: vault.name, id: vault.id, fromICloud: false,
                                                         mounted: vault.mounted, hasACopy: vault.hasACopy))
                    }
                    Spacer()
                    Menu("Więcej") {
                        Button("Sprawdź spójność (fsck)") { Task { await model.fsck(vault) } }
                            .disabled(vault.card == nil || vault.mounted)
                        Button("Pokaż log montowania") { model.revealLog(vault) }
                    }
                    .fixedSize()
                }
                .disabled(model.busy != nil)
            }
            Section {
                Text("Maski: \(model.state.masks)")
                    .font(.caption).foregroundStyle(.secondary)
                Text("Odmontuj przed wyjęciem karty. Wyjęcie karty przy zamontowanym sejfie blokuje wolumin, a niezapisane zmiany przepadają.")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
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
