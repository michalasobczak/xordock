// swift-tools-version:5.10
// Aplikacja xordock (S007, S009): pasek menu + okno. Silnik to dołączony CLI xordock.
import PackageDescription

let package = Package(
    name: "Xordock",
    platforms: [.macOS(.v14)],
    targets: [
        .target(name: "XordockCore"),
        .target(name: "XordockUI", dependencies: ["XordockCore"]),
        .executableTarget(name: "XordockApp", dependencies: ["XordockCore", "XordockUI"]),
        // Render pulpitu do PNG dla przykładowych stanów (S009 G4).
        .executableTarget(name: "XordockPreview", dependencies: ["XordockCore", "XordockUI"]),
        .testTarget(name: "XordockCoreTests", dependencies: ["XordockCore"]),
    ]
)
