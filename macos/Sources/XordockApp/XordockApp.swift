import AppKit
import SwiftUI
import XordockCore

@main
struct XordockApp: App {
    @State private var model = AppModel()

    var body: some Scene {
        MenuBarExtra {
            MenuContent(model: model)
        } label: {
            Image(systemName: iconName)
        }
        .menuBarExtraStyle(.menu)

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
                Text(v.statusLine)
                if v.card != nil, v.backupDue {
                    Text(v.lastBackup == nil ? "⚠︎ Brak backupu" : "⚠︎ Backup starszy niż 7 dni")
                } else if v.card != nil, v.lastBackup != nil, !v.hasACopy {
                    Text("⚠︎ Brak kopii udziału A w iCloud")
                }
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
        Button("Zakończ") { NSApp.terminate(nil) }
            .keyboardShortcut("q")
    }

    private func open() {
        openWindow(id: "main")
        NSApp.activate(ignoringOtherApps: true)
    }
}
