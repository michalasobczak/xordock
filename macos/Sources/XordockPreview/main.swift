import AppKit
import SwiftUI
import XordockCore
import XordockUI

// Render pulpitu do PNG dla przykładowych stanów (S009 G4).
// Użycie: swift run XordockPreview <katalog wyjściowy>

let out = URL(fileURLWithPath: CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : ".")
try? FileManager.default.createDirectory(at: out, withIntermediateDirectories: true)

func vault(_ patch: (inout [String: Any]) -> Void) -> VaultInfo {
    var d: [String: Any] = [
        "name": "test", "id": "517f", "a": "/Users/u/Library/Application Support/" + "xordock/test",
        "card": "/Volumes/xor_primary/xordock/test",
        "spares": [["root": "/Volumes/xor_backup/xordock/test", "volume": "/Volumes/xor_backup", "media": "m"]],
        "mounted": false, "mountpoint": "/Volumes/xordock-test", "seq": 41, "snapshots": 2,
        "lastBackup": ["time": "2026-11-22T18:21:00Z", "point": "p", "snapshot": "s", "seq": 30, "media": "m"],
        "changesSinceBackup": 3, "backupDue": false,
        "aCopy": ["name": "test", "id": "517f", "path": "/Users/u/Library/Mobile Documents/com~apple~CloudDocs/xordock/test",
                  "points": 1, "lastPoint": "2026-11-22T18:21:00Z"],
    ]
    patch(&d)
    let data = try! JSONSerialization.data(withJSONObject: d)
    return try! jsonDecoder.decode(VaultInfo.self, from: data)
}

let cases: [(String, VaultInfo, Bool)] = [
    ("1-ok", vault { _ in }, false),
    ("2-brak-kopii-A", vault { $0["aCopy"] = nil }, false),
    ("3-brak-karty", vault { $0["card"] = nil; $0["spares"] = []; $0["lastBackup"] = nil; $0["aCopy"] = nil }, false),
    ("4-otwarty-szczegoly", vault { $0["mounted"] = true; $0["changesSinceBackup"] = 0 }, true),
]

@MainActor func render() {
    for scheme in [ColorScheme.light, .dark] {
        for (name, v, details) in cases {
            let view = Dashboard(vault: v, icloud: true, masks: "CSPRNG macOS (SecRandomCopyBytes): ochrona praktyczna (obliczeniowa), BEZ gwarancji informacyjno-teoretycznej",
                                 busy: false, actions: DashboardActions(), showDetails: details).content
                .frame(width: name.hasPrefix("3") ? 560 : 760, alignment: .topLeading)
                .background(scheme == .dark ? Color(white: 0.12) : Color(white: 0.98))
                .environment(\.colorScheme, scheme)
            let r = ImageRenderer(content: view)
            r.scale = 1
            guard let img = r.nsImage, let tiff = img.tiffRepresentation,
                  let png = NSBitmapImageRep(data: tiff)?.representation(using: .png, properties: [:]) else {
                print("błąd renderu \(name)")
                continue
            }
            let f = out.appendingPathComponent("\(name)-\(scheme == .dark ? "ciemny" : "jasny").png")
            try! png.write(to: f)
            print(f.path)
        }
    }
}

MainActor.assumeIsolated { render() }
