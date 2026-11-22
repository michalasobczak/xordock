import AppKit
import SwiftUI
import XordockCore
import XordockUI

@main
struct XordockApp: App {
    @State private var model = AppModel()

    var body: some Scene {
        MenuBarExtra {
            MenuContent(model: model)
        } label: {
            MenuBarLabel(model: model, iconName: iconName)
        }
        .menuBarExtraStyle(.menu)

        Settings {
            SettingsView(model: model)
        }

        Window("xordock", id: "main") {
            MainView(model: model)
                .frame(minWidth: 760, minHeight: 480)
        }
        .defaultSize(width: 900, height: 560)
    }

    private var iconName: String {
        switch model.state.attention {
        case .ok: return "lock.shield"
        case .mounted: return "lock.open"
        case .warning: return "exclamationmark.shield"
        }
    }
}

struct MenuContent: View {
    let model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        if model.cliMissing {
            Text("Brak programu xordock w pakiecie aplikacji")
        }
        if !model.state.fileVault {
            Text("⚠︎ FileVault wyłączony")
        }
        ForEach(model.state.orphanCopies) { c in
            Text("⚠︎ \(c.name): kopia A w iCloud bez SSD. Otwórz okno, aby odzyskać")
        }
        ForEach(model.state.vaults) { v in
            Section(v.name) {
                let h = VaultHealth.evaluate(v, icloud: model.state.icloud)
                Text((h.level == .ok ? "✓ " : "⚠︎ ") + h.title)
                if v.mounted {
                    Button("Pokaż w Finderze") { model.showInFinder(v.mountpoint) }
                    Button("Odmontuj") { Task { _ = await model.unmount(v) } }
                } else if v.card != nil, v.error == nil {
                    Button("Zamontuj") { Task { await model.mount(v) } }
                }
            }
        }
        if let b = model.busy { Text(b) }
        Divider()
        Button("Otwórz xordock…") { open() }
            .keyboardShortcut("o")
        Button("Samouczek…") {
            model.pendingIntro = true
            open()
        }
        Button("Wymagania…") {
            model.pendingRequirements = true
            open()
        }
        SettingsLink { Text("Ustawienia…") }
            .keyboardShortcut(",")
        Button("Zakończ") { NSApp.terminate(nil) }
            .keyboardShortcut("q")
    }

    private func open() {
        openWindow(id: "main")
        NSApp.activate(ignoringOtherApps: true)
    }
}

/// Ikona w pasku menu. Jest rysowana od startu aplikacji, więc tu sprawdzamy
/// wymagania (S011) i otwieramy okno przy brakach albo z pytaniem o samouczek (S010).
struct MenuBarLabel: View {
    let model: AppModel
    let iconName: String
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        Image(systemName: iconName)
            .task {
                if await model.startup() {
                    openWindow(id: "main")
                    NSApp.activate(ignoringOtherApps: true)
                }
            }
    }
}

/// Ustawienia (⌘,), S010 T-02.
struct SettingsView: View {
    let model: AppModel
    @AppStorage(AppModel.askKey) private var askAtLaunch = true
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        Form {
            Section("Wymagania") {
                Button("Sprawdź wymagania (macFUSE, rozszerzenie, FileVault, iCloud)") {
                    model.pendingRequirements = true
                    openWindow(id: "main")
                    NSApp.activate(ignoringOtherApps: true)
                    Task { await model.checkRequirements(forceProbe: true) }
                }
            }
            Section("Samouczek") {
                Toggle("Pytaj o samouczek przy starcie", isOn: $askAtLaunch)
                Button("Uruchom samouczek teraz") {
                    model.pendingIntro = true
                    openWindow(id: "main")
                    NSApp.activate(ignoringOtherApps: true)
                }
            }
        }
        .formStyle(.grouped)
        .frame(width: 420)
        .padding()
    }
}
