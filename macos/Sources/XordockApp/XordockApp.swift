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
            Text("The xordock program is missing from the app bundle")
        }
        if !model.state.fileVault {
            Text("⚠︎ FileVault is off")
        }
        ForEach(model.state.orphanCopies) { c in
            Text("⚠︎ \(c.name): share A copy in iCloud without SSD. Open the window to recover")
        }
        ForEach(model.state.vaults) { v in
            Section(v.name) {
                let h = VaultHealth.evaluate(v, icloud: model.state.icloud)
                Text((h.level == .ok ? "✓ " : "⚠︎ ") + h.title)
                if v.mounted {
                    Button("Show in Finder") { model.showInFinder(v.mountpoint) }
                    Button("Unmount") { Task { _ = await model.unmount(v) } }
                } else if v.card != nil, v.error == nil {
                    Button("Mount") { Task { await model.mount(v) } }
                }
            }
        }
        if let b = model.busy { Text(b) }
        Divider()
        Button("Open xordock…") { open() }
            .keyboardShortcut("o")
        Button("Tutorial…") {
            model.pendingIntro = true
            open()
        }
        Button("Requirements…") {
            model.pendingRequirements = true
            open()
        }
        SettingsLink { Text("Settings…") }
            .keyboardShortcut(",")
        Button("Quit") { NSApp.terminate(nil) }
            .keyboardShortcut("q")
    }

    private func open() {
        openWindow(id: "main")
        NSApp.activate(ignoringOtherApps: true)
    }
}

/// Menu bar icon. It is drawn from app launch, so here we check
/// requirements (S011) and open the window when something is missing or with the tutorial prompt (S010).
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

/// Settings (⌘,), S010 T-02.
struct SettingsView: View {
    let model: AppModel
    @AppStorage(AppModel.askKey) private var askAtLaunch = true
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        Form {
            Section("Requirements") {
                Button("Check requirements (macFUSE, extension, FileVault, iCloud)") {
                    model.pendingRequirements = true
                    openWindow(id: "main")
                    NSApp.activate(ignoringOtherApps: true)
                    Task { await model.checkRequirements(forceProbe: true) }
                }
            }
            Section("Tutorial") {
                Toggle("Ask about the tutorial at launch", isOn: $askAtLaunch)
                Button("Start tutorial now") {
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
