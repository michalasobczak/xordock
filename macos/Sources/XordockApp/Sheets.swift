import AppKit
import SwiftUI
import XordockCore
import XordockUI

struct NewVaultSheet: View {
    let model: AppModel
    @Environment(TourController.self) private var tour: TourController?
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var card: VolumeInfo?

    private var cards: [VolumeInfo] { model.volumes.filter(\.usableAsCard) }
    private var nameError: String? {
        if let e = VaultName.validate(name) { return e }
        if model.state.vaults.contains(where: { $0.name == name }) { return "A vault with this name already exists." }
        return nil
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("New vault").font(.title2)
            if tour?.step == .createVault {
                TourHint("Enter a name, e.g. \"Documents\", and choose a card or USB drive. Half B will be stored on it.")
            }
            Form {
                TextField("Name", text: $name)
                if !name.isEmpty, let e = nameError { Text(e).foregroundStyle(.red).font(.caption) }
                VolumePicker(title: "Working card (B)", volumes: cards, selection: $card)
                if cards.isEmpty {
                    Text("Connect an SD card or USB drive (preferably APFS or Mac OS Extended).").foregroundStyle(.orange)
                }
                if let c = card, c.fs == "msdos" || c.fs == "exfat" {
                    Text("FAT/exFAT copes worse with sudden removal. APFS or Mac OS Extended is recommended.").foregroundStyle(.orange).font(.caption)
                }
            }
            .formStyle(.grouped)
            VStack(alignment: .leading, spacing: 4) {
                Text("Share A goes to this Mac's SSD, share B to the chosen card. Neither of them on its own contains file contents.")
                Text("Practical (computational) protection, NO information-theoretic guarantee.")
                Text("Without the working card the data can't be read. Back up regularly to a second card.")
                    .foregroundStyle(.orange)
            }
            .font(.caption)
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Create vault") {
                    guard let c = card else { return }
                    Task { if await model.create(name: name, card: c) { dismiss() } }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(nameError != nil || card == nil || model.busy != nil)
            }
            if let e = model.errorText { Text(e).foregroundStyle(.red).textSelection(.enabled) }
        }
        .padding(20)
        .frame(width: 560)
        .task { await model.refresh() }
    }
}

struct BackupSheet: View {
    let model: AppModel
    @Environment(TourController.self) private var tour: TourController?
    let vault: VaultInfo
    @Environment(\.dismiss) private var dismiss
    @State private var spare: VolumeInfo?
    @State private var showADP = false

    var body: some View {
        let cands = vault.candidatesForSpare(in: model.volumes)
        VStack(alignment: .leading, spacing: 12) {
            Text("Back up vault \"\(vault.name)\"").font(.title2)
            if tour?.step == .backup {
                TourHint("Choose a second drive (not the working card) and click \"Run backup\".")
            }
            Form {
                VolumePicker(title: "Spare card", volumes: cands, selection: $spare)
                if cands.isEmpty {
                    Text("Connect a second drive (not the working card).").foregroundStyle(.orange)
                }
            }
            .formStyle(.grouped)
            VStack(alignment: .leading, spacing: 4) {
                if vault.mounted {
                    Text("The vault will be unmounted (save and close open files).").foregroundStyle(.orange)
                }
                Text("Snapshot → copy of B to the spare card → copy of A to iCloud → verification.")
                if !model.state.icloud {
                    Text("iCloud Drive unavailable: only the copy of B will be made. Losing the SSD = losing data.").foregroundStyle(.orange)
                }
                Text("The working card and the spare card must be connected at the same time.")
            }
            .font(.caption)
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Run backup") {
                    if model.state.icloud && !model.adpAccepted {
                        showADP = true
                    } else {
                        run()
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(spare == nil || model.busy != nil)
            }
            if let b = model.busy { HStack { ProgressView().controlSize(.small); Text(b) } }
            if let e = model.errorText { Text(e).foregroundStyle(.red).textSelection(.enabled) }
        }
        .padding(20)
        .frame(width: 560)
        .alert("Share A copy in iCloud", isPresented: $showADP) {
            Button("I understand, continue") {
                model.adpAccepted = true
                run()
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Share A will go to iCloud Drive. On its own it reveals nothing, but the iCloud account together with any B card (working or spare) is a complete set. Enable two-factor authentication and Advanced Data Protection (System Settings → [Your name] → iCloud) and keep the cards apart.")
        }
        .task {
            await model.refresh()
            spare = vault.defaultSpare(in: model.volumes)
        }
    }

    private func run() {
        guard let s = spare else { return }
        Task { if await model.backup(vault, spare: s) { dismiss() } }
    }
}

struct RecoverySheet: View {
    let model: AppModel
    let target: RecoveryTarget
    @Environment(\.dismiss) private var dismiss
    @State private var spare: VolumeInfo?
    @State private var result: PointsResult?
    @State private var point: PointInfo?
    @State private var newCard: VolumeInfo?
    @State private var newName = ""
    @State private var loadError: String?
    @State private var fromICloud = false

    var body: some View {
        let spares = recoverySources(id: target.id, volumes: model.volumes, fromICloud: fromICloud)
        VStack(alignment: .leading, spacing: 12) {
            Text("Recover vault \"\(target.name)\"").font(.title2)
            Text(fromICloud
                 ? "Share A from the copy in iCloud Drive (without SSD), share B from the spare or working card."
                 : "Works without the working card: rebuilds the data from share A on this Mac and the spare card.")
                .font(.caption)
            Form {
                if target.hasACopy && !target.fromICloud {
                    Toggle("Share A from the iCloud copy (when A on the SSD is damaged)", isOn: $fromICloud)
                }
                VolumePicker(title: fromICloud ? "Card (spare or working)" : "Spare card", volumes: spares, selection: $spare)
                if spares.isEmpty {
                    Text("No card of this vault was detected.").foregroundStyle(.orange)
                }
                if let r = result {
                    Picker("Recovery point", selection: $point) {
                        ForEach(r.points) { p in
                            Text("\(p.created.formatted(date: .abbreviated, time: .shortened)) · commit \(p.seq) · \(formatBytes(UInt64(max(p.bytes, 0))))")
                                .tag(PointInfo?.some(p))
                        }
                    }
                    ForEach(r.rejected, id: \.self) { Text("Rejected: \($0)").font(.caption).foregroundStyle(.orange) }
                    if r.points.isEmpty { Text("No READY points.").foregroundStyle(.red) }
                }
                if let e = loadError { Text(e).foregroundStyle(.red) }
            }
            .formStyle(.grouped)
            .onChange(of: spare) { Task { await load() } }
            .onChange(of: fromICloud) {
                spare = recoverySources(id: target.id, volumes: model.volumes, fromICloud: fromICloud).first
                Task { await load() }
            }

            if let s = spare, let p = point {
                GroupBox("Check") {
                    HStack {
                        Text("Rebuilds and checks all files of the point, writes nothing.").font(.caption)
                        Spacer()
                        Button("Verify") {
                            Task { await model.perform("Verifying point…", [Commands.verify(name: target.name, spare: s.path, point: p.id, fromICloud: fromICloud)], success: "Point complete: all files can be restored.") }
                        }
                    }
                }
                GroupBox("Export plaintext files") {
                    HStack {
                        Text("Files in the chosen folder are NOT protected by the XOR split.").font(.caption).foregroundStyle(.orange)
                        Spacer()
                        Button("Export to…") { export(spare: s, point: p) }
                    }
                }
                GroupBox("Restore to a new card") {
                    VStack(alignment: .leading) {
                        VolumePicker(title: "New working card", volumes: model.volumes.filter { $0.usableAsCard && $0.path != s.path }, selection: $newCard)
                        TextField("New vault name", text: $newName)
                        if !newName.isEmpty, let e = VaultName.validate(newName) { Text(e).font(.caption).foregroundStyle(.red) }
                        HStack {
                            Text("The new vault gets new masks. After checking it, delete the old share A.").font(.caption)
                            Spacer()
                            Button("Restore") {
                                guard let c = newCard else { return }
                                Task {
                                    await model.perform("Restoring…", [Commands.restore(name: target.name, spare: s.path, point: p.id, card: c.path, newName: newName, fromICloud: fromICloud)], success: "Restored vault \"\(newName)\". Check it, then delete all copies of the old A.")
                                }
                            }
                            .disabled(newCard == nil || VaultName.validate(newName) != nil)
                        }
                    }
                }
            }
            HStack {
                if let b = model.busy { ProgressView().controlSize(.small); Text(b) }
                Spacer()
                Button("Close") { dismiss() }.keyboardShortcut(.cancelAction)
            }
            if let e = model.errorText { Text(e).foregroundStyle(.red).textSelection(.enabled) }
            if let m = model.message, !m.isEmpty { Text(m).foregroundStyle(.green).textSelection(.enabled) }
        }
        .disabled(model.busy != nil)
        .padding(20)
        .frame(width: 620)
        .task {
            fromICloud = target.fromICloud
            await model.refresh()
            // Without share A on the Mac the name can stay the same.
            newName = target.fromICloud ? target.name : target.name + "-recovered"
            if spare == nil { spare = recoverySources(id: target.id, volumes: model.volumes, fromICloud: fromICloud).first }
        }
    }

    private func load() async {
        result = nil
        point = nil
        loadError = nil
        guard let s = spare else { return }
        if target.mounted && !fromICloud {
            loadError = "Unmount the vault before recovery."
            return
        }
        do {
            let r = try await model.points(name: target.name, spare: s, fromICloud: fromICloud)
            result = r
            point = r.points.first
        } catch {
            loadError = error.localizedDescription
        }
    }

    private func export(spare: VolumeInfo, point: PointInfo) {
        let panel = NSSavePanel()
        panel.title = "New folder for exported files"
        panel.nameFieldStringValue = "\(target.name)-export"
        panel.canCreateDirectories = true
        guard panel.runModal() == .OK, let url = panel.url else { return }
        Task {
            await model.perform("Exporting…", [Commands.export(name: target.name, spare: spare.path, point: point.id, to: url.path, fromICloud: fromICloud)], success: "Exported to \(url.lastPathComponent) (plaintext files).")
        }
    }
}
