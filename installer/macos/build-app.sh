#!/bin/sh
# Builds "Varu Installer.app" for Apple Silicon and Intel, zipped for a
# release: sh installer/macos/build-app.sh v1.2.3 dist
# Needs macOS (lipo, codesign, ditto) and Go.
set -eu
version=${1:-dev}
mkdir -p "${2:-dist}"
out=$(cd "${2:-dist}" && pwd)
cd "$(dirname "$0")/.."

app="$out/Varu Installer.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS"
for arch in arm64 amd64; do
  GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$out/varu-installer-$arch" .
done
lipo -create -output "$app/Contents/MacOS/varu-installer" "$out/varu-installer-arm64" "$out/varu-installer-amd64"
rm "$out/varu-installer-arm64" "$out/varu-installer-amd64"
sed "s/@VERSION@/${version#v}/g" macos/Info.plist > "$app/Contents/Info.plist"

# Ad-hoc signature: enough to run on Apple Silicon, not enough for
# Gatekeeper. A Developer ID signature and notarisation remove the
# "cannot verify" warning; see README.
codesign --force --sign - "$app"
rm -f "$out/Varu-Installer-macOS.zip"
ditto -c -k --keepParent "$app" "$out/Varu-Installer-macOS.zip"
echo "wrote $out/Varu-Installer-macOS.zip"
