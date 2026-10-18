#!/bin/sh
# Builds dist/Kennel.app: the Swift shell plus the kennel server inside its Resources.
set -e
cd "$(dirname "$0")"
VERSION="${VERSION:-dev}"
DIST="${DIST:-$(pwd)/../dist}"
APP="$DIST/Kennel.app"
swift build -c release
rm -rf "$APP" && mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp .build/release/Kennel "$APP/Contents/MacOS/Kennel"
(cd .. && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$VERSION" -o "$APP/Contents/Resources/kennel" ./cmd/kennel)
ICONSET="$(mktemp -d)/Kennel.iconset"
swift scripts/icon.swift "$ICONSET"
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/Kennel.icns"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Kennel</string>
  <key>CFBundleDisplayName</key><string>Kennel</string>
  <key>CFBundleIdentifier</key><string>dev.razkevich.kennel</string>
  <key>CFBundleExecutable</key><string>Kennel</string>
  <key>CFBundleIconFile</key><string>Kennel</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>LSMinimumSystemVersion</key><string>13.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSSupportsAutomaticTermination</key><false/>
</dict></plist>
PLIST
codesign --force --deep --sign - "$APP"
echo "$APP"
