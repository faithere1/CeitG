#!/bin/sh
# Builds CeitG for macOS (universal .app) and Windows (.exe) into dist/.
# Run from anywhere on a Mac with Go installed:  sh scripts/build.sh
set -e
cd "$(dirname "$0")/.."
rm -rf dist && mkdir -p dist

echo "→ Windows"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/CeitG.exe .

echo "→ macOS"
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/ceitg-arm64 .
GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o dist/ceitg-amd64 .
APP=dist/CeitG.app/Contents
mkdir -p "$APP/MacOS" "$APP/Resources"
lipo -create -output "$APP/MacOS/CeitG" dist/ceitg-arm64 dist/ceitg-amd64
rm dist/ceitg-arm64 dist/ceitg-amd64
go run ./scripts/mkicon dist/icon.iconset
iconutil -c icns dist/icon.iconset -o "$APP/Resources/CeitG.icns"
rm -rf dist/icon.iconset
cat > "$APP/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>CeitG</string>
  <key>CFBundleDisplayName</key><string>CeitG</string>
  <key>CFBundleIdentifier</key><string>io.github.ceitg</string>
  <key>CFBundleExecutable</key><string>CeitG</string>
  <key>CFBundleIconFile</key><string>CeitG</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleVersion</key><string>1.0</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>LSMinimumSystemVersion</key><string>10.15</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
xattr -cr dist/CeitG.app   # stray attributes make codesign fail
codesign --force --deep -s - dist/CeitG.app
(cd dist && zip -qry CeitG-macOS.zip CeitG.app && zip -q CeitG-Windows.zip CeitG.exe)
ls -lh dist
