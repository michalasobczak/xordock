import AppKit
import SwiftUI
import XordockCore
import XordockUI

// Renders the dashboard to PNG for sample states (S009 G4).
// Usage: swift run XordockPreview <output directory>

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
    ("2-no-a-copy", vault { $0["aCopy"] = nil }, false),
    ("3-no-card", vault { $0["card"] = nil; $0["spares"] = []; $0["lastBackup"] = nil; $0["aCopy"] = nil }, false),
    ("4-open-details", vault { $0["mounted"] = true; $0["changesSinceBackup"] = 0 }, true),
]

@MainActor func render() {
    for scheme in [ColorScheme.light, .dark] {
        for (name, v, details) in cases {
            let view = Dashboard(vault: v, icloud: true, masks: "CSPRNG macOS (SecRandomCopyBytes): practical (computational) protection, NO information-theoretic guarantee",
                                 busy: false, actions: DashboardActions(), showDetails: details).content
                .frame(width: name.hasPrefix("3") ? 560 : 760, alignment: .topLeading)
                .background(scheme == .dark ? Color(white: 0.12) : Color(white: 0.98))
                .environment(\.colorScheme, scheme)
            let r = ImageRenderer(content: view)
            r.scale = 1
            guard let img = r.nsImage, let tiff = img.tiffRepresentation,
                  let png = NSBitmapImageRep(data: tiff)?.representation(using: .png, properties: [:]) else {
                print("render error \(name)")
                continue
            }
            let f = out.appendingPathComponent("\(name)-\(scheme == .dark ? "dark" : "light").png")
            try! png.write(to: f)
            print(f.path)
        }
    }
}

@MainActor func save<V: View>(_ v: V, _ name: String, dark: Bool) {
    let view = v.background(dark ? Color(white: 0.16) : Color(white: 0.98)).environment(\.colorScheme, dark ? .dark : .light)
    let r = ImageRenderer(content: view)
    r.scale = 1
    guard let img = r.nsImage, let tiff = img.tiffRepresentation,
          let png = NSBitmapImageRep(data: tiff)?.representation(using: .png, properties: [:]) else { return print("error \(name)") }
    let f = out.appendingPathComponent("\(name)-\(dark ? "dark" : "light").png")
    try! png.write(to: f)
    print(f.path)
}

// S010: introduction screens, tips, launch prompt.
@MainActor func renderTutorial() {
    for dark in [false, true] {
        for i in Intro.pages.indices {
            save(IntroView(start: i, onFinish: {}, onSkip: {}), "intro-\(i + 1)", dark: dark)
        }
        for s in [TourStep.mount, .backup, .menuBar] {
            save(TourBubble(step: s, number: s.rawValue + 1, onNext: {}, onFinish: {}), "tip-\(s)", dark: dark)
        }
        save(StartPrompt(askAtLaunch: .constant(true), onStart: {}, onLater: {}), "start", dark: dark)
    }
}

// S011: requirements sheet with missing items.
@MainActor func renderRequirements() {
    let r = DoctorResult(macfuseVersion: nil, checks: [
        RequirementCheck(id: "macos", title: "macOS 26 or later", level: "ok", detail: "macOS 27.0"),
        RequirementCheck(id: "macfuse", title: "macFUSE 5.5 or later", level: "error",
                         detail: "macFUSE is not installed. Without it the vault can't be opened in Finder.",
                         fixLabel: "Download macFUSE", fixURL: "https://macfuse.github.io/"),
        RequirementCheck(id: "extension", title: "macFUSE file system extension", level: "unknown",
                         detail: "Not checked. Checking requires a test mount."),
        RequirementCheck(id: "filevault", title: "FileVault", level: "ok", detail: "Enabled."),
        RequirementCheck(id: "icloud", title: "iCloud Drive", level: "warning",
                         detail: "Unavailable. Backup won't make a copy of share A, so losing the Mac = losing data.",
                         fixLabel: "Open iCloud settings", fixURL: "x-apple.systempreferences:com.apple.systempreferences.AppleIDSettings:icloud"),
    ])
    for dark in [false, true] {
        save(RequirementsView(result: r, checking: false, onRecheck: {}, onClose: {}), "requirements", dark: dark)
    }
}

MainActor.assumeIsolated {
    render()
    renderTutorial()
    renderRequirements()
}
