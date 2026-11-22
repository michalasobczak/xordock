// swift-tools-version:5.10
// Aplikacja xordock (S007): pasek menu + okno. Silnik to dołączony CLI xordock.
import PackageDescription

let package = Package(
    name: "Xordock",
    platforms: [.macOS(.v14)],
    targets: [
        .target(name: "XordockCore"),
        .executableTarget(name: "XordockApp", dependencies: ["XordockCore"]),
        .testTarget(name: "XordockCoreTests", dependencies: ["XordockCore"]),
    ]
)
