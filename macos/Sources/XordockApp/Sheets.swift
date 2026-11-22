import AppKit
import SwiftUI
import XordockCore

struct NewVaultSheet: View {
    let model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var card: VolumeInfo?

    private var cards: [VolumeInfo] { model.volumes.filter(\.usableAsCard) }
    private var nameError: String? {
        if let e = VaultName.validate(name) { return e }
        if model.state.vaults.contains(where: { $0.name == name }) { return "Sejf o tej nazwie już istnieje." }
        return nil
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Nowy sejf").font(.title2)
            Form {
                TextField("Nazwa", text: $name)
                if !name.isEmpty, let e = nameError { Text(e).foregroundStyle(.red).font(.caption) }
                VolumePicker(title: "Karta robocza (B)", volumes: cards, selection: $card)
                if cards.isEmpty {
                    Text("Podłącz kartę SD albo pendrive (najlepiej APFS lub Mac OS Extended).").foregroundStyle(.orange)
                }
                if let c = card, c.fs == "msdos" || c.fs == "exfat" {
                    Text("FAT/exFAT gorzej znosi nagłe wyjęcie. Zalecany APFS lub Mac OS Extended.").foregroundStyle(.orange).font(.caption)
                }
            }
            .formStyle(.grouped)
            VStack(alignment: .leading, spacing: 4) {
                Text("Udział A trafi na SSD tego Maca, udział B na wybraną kartę. Żaden z nich osobno nie zawiera treści plików.")
                Text("Ochrona jest praktyczna (obliczeniowa), BEZ gwarancji informacyjno-teoretycznej.")
                Text("Bez karty roboczej danych nie da się odczytać. Rób regularnie backup na drugą kartę.")
                    .foregroundStyle(.orange)
            }
            .font(.caption)
            HStack {
                Spacer()
                Button("Anuluj") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Załóż sejf") {
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
    let vault: VaultInfo
    @Environment(\.dismiss) private var dismiss
    @State private var spare: VolumeInfo?
    @State private var showADP = false

    var body: some View {
        let cands = vault.candidatesForSpare(in: model.volumes)
        VStack(alignment: .leading, spacing: 12) {
            Text("Backup sejfu „\(vault.name)”").font(.title2)
            Form {
                VolumePicker(title: "Karta zapasowa", volumes: cands, selection: $spare)
                if cands.isEmpty {
                    Text("Podłącz drugi nośnik (inny niż karta robocza).").foregroundStyle(.orange)
                }
            }
            .formStyle(.grouped)
            VStack(alignment: .leading, spacing: 4) {
                if vault.mounted {
                    Text("Sejf zostanie odmontowany (zapisz i zamknij otwarte pliki).").foregroundStyle(.orange)
                }
                Text("Migawka → kopia B na kartę zapasową → kopia A do iCloud → weryfikacja.")
                if !model.state.icloud {
                    Text("iCloud Drive niedostępny: powstanie tylko kopia B. Utrata SSD = utrata danych.").foregroundStyle(.orange)
                }
                Text("Karta robocza i zapasowa muszą być podłączone jednocześnie.")
            }
            .font(.caption)
            HStack {
                Spacer()
                Button("Anuluj") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Wykonaj backup") {
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
        .alert("Kopia udziału A w iCloud", isPresented: $showADP) {
            Button("Rozumiem, kontynuuj") {
                model.adpAccepted = true
                run()
            }
            Button("Anuluj", role: .cancel) {}
        } message: {
            Text("Udział A trafi do iCloud Drive. Sam niczego nie zdradza, ale konto iCloud razem z dowolną kartą B (roboczą lub zapasową) daje komplet. Włącz dwuetapowe uwierzytelnianie i Advanced Data Protection (Ustawienia → [Twoje imię] → iCloud) i nie trzymaj kart razem.")
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
            Text("Odzyskiwanie sejfu „\(target.name)”").font(.title2)
            Text(fromICloud
                 ? "Udział A z kopii w iCloud Drive (bez SSD), udział B z karty zapasowej albo roboczej."
                 : "Działa bez karty roboczej: składa dane z udziału A na tym Macu i karty zapasowej.")
                .font(.caption)
            Form {
                if target.hasACopy && !target.fromICloud {
                    Toggle("Udział A z kopii w iCloud (gdy A na SSD jest uszkodzony)", isOn: $fromICloud)
                }
                VolumePicker(title: fromICloud ? "Karta (zapasowa lub robocza)" : "Karta zapasowa", volumes: spares, selection: $spare)
                if spares.isEmpty {
                    Text("Nie wykryto karty tego sejfu.").foregroundStyle(.orange)
                }
                if let r = result {
                    Picker("Punkt odzyskiwania", selection: $point) {
                        ForEach(r.points) { p in
                            Text("\(p.created.formatted(date: .abbreviated, time: .shortened)) · commit \(p.seq) · \(formatBytes(UInt64(max(p.bytes, 0))))")
                                .tag(PointInfo?.some(p))
                        }
                    }
                    ForEach(r.rejected, id: \.self) { Text("Odrzucony: \($0)").font(.caption).foregroundStyle(.orange) }
                    if r.points.isEmpty { Text("Brak punktów READY.").foregroundStyle(.red) }
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
                GroupBox("Sprawdź") {
                    HStack {
                        Text("Składa i sprawdza wszystkie pliki punktu, niczego nie zapisuje.").font(.caption)
                        Spacer()
                        Button("Weryfikuj") {
                            Task { await model.perform("Weryfikacja punktu…", [Commands.verify(name: target.name, spare: s.path, point: p.id, fromICloud: fromICloud)], success: "Punkt kompletny: wszystkie pliki dają się odtworzyć.") }
                        }
                    }
                }
                GroupBox("Eksport jawnych plików") {
                    HStack {
                        Text("Pliki w wybranym katalogu NIE są chronione podziałem XOR.").font(.caption).foregroundStyle(.orange)
                        Spacer()
                        Button("Eksportuj do…") { export(spare: s, point: p) }
                    }
                }
                GroupBox("Odtwórz na nową kartę") {
                    VStack(alignment: .leading) {
                        VolumePicker(title: "Nowa karta robocza", volumes: model.volumes.filter { $0.usableAsCard && $0.path != s.path }, selection: $newCard)
                        TextField("Nazwa nowego sejfu", text: $newName)
                        if !newName.isEmpty, let e = VaultName.validate(newName) { Text(e).font(.caption).foregroundStyle(.red) }
                        HStack {
                            Text("Nowy sejf dostaje nowe maski. Po sprawdzeniu usuń stary udział A.").font(.caption)
                            Spacer()
                            Button("Odtwórz") {
                                guard let c = newCard else { return }
                                Task {
                                    await model.perform("Odtwarzanie…", [Commands.restore(name: target.name, spare: s.path, point: p.id, card: c.path, newName: newName, fromICloud: fromICloud)], success: "Odtworzono sejf „\(newName)”. Sprawdź go, potem usuń wszystkie kopie starego A.")
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
                Button("Zamknij") { dismiss() }.keyboardShortcut(.cancelAction)
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
            // Bez udziału A na Macu nazwa może zostać ta sama.
            newName = target.fromICloud ? target.name : target.name + "-odzyskany"
            if spare == nil { spare = recoverySources(id: target.id, volumes: model.volumes, fromICloud: fromICloud).first }
        }
    }

    private func load() async {
        result = nil
        point = nil
        loadError = nil
        guard let s = spare else { return }
        if target.mounted && !fromICloud {
            loadError = "Odmontuj sejf przed odzyskiwaniem."
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
        panel.title = "Nowy katalog na wyeksportowane pliki"
        panel.nameFieldStringValue = "\(target.name)-eksport"
        panel.canCreateDirectories = true
        guard panel.runModal() == .OK, let url = panel.url else { return }
        Task {
            await model.perform("Eksport…", [Commands.export(name: target.name, spare: spare.path, point: point.id, to: url.path, fromICloud: fromICloud)], success: "Wyeksportowano do \(url.lastPathComponent) (pliki jawne).")
        }
    }
}
