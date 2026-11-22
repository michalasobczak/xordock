#!/bin/zsh
# Assembles build/xordock.app: the SwiftUI app (macos/) + the xordock CLI in
# Contents/Resources. Ad hoc signature, no Apple account needed (S007 G3).
# Requires: Go, Xcode (Swift 6), macFUSE 5.5 (libfuse3 in /usr/local) to mount.
set -euo pipefail
cd "${0:A:h}/.."
export DEVELOPER_DIR="${DEVELOPER_DIR:-/Applications/Xcode.app/Contents/Developer}"

go build -o bin/xordock ./cmd/xordock
(cd macos && swift build -c release --product XordockApp)

app=build/xordock.app
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp macos/.build/release/XordockApp "$app/Contents/MacOS/XordockApp"
cp bin/xordock "$app/Contents/Resources/xordock"
version=$(git describe --always --dirty 2>/dev/null || echo dev)
cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key><string>local.xordock.app</string>
  <key>CFBundleName</key><string>xordock</string>
  <key>CFBundleDisplayName</key><string>xordock</string>
  <key>CFBundleExecutable</key><string>XordockApp</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>0.7</string>
  <key>CFBundleVersion</key><string>${version}</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
codesign --force --sign - "$app/Contents/Resources/xordock"
codesign --force --sign - "$app"
codesign --verify --strict "$app"
echo "Built: $app"
