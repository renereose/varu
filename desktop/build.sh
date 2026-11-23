#!/bin/sh
# Builds the Varu desktop app for macOS, Windows and Linux and copies the
# downloads into dist/:  sh desktop/build.sh v1.2.3 dist
# Needs macOS (the Mac app can only be built on a Mac), Go and Node.
set -eu
version=${1:-dev}
mkdir -p "${2:-dist}"
out=$(cd "${2:-dist}" && pwd)
here=$(cd "$(dirname "$0")" && pwd)

# The installer, which the app runs when there is no Varu data yet.
bin="$here/bin"
rm -rf "$bin"
mkdir -p "$bin/mac-x64" "$bin/mac-arm64" "$bin/win-x64" "$bin/linux-x64" "$bin/linux-arm64"
build() {
  (cd "$here/../installer" && GOOS=$1 GOARCH=$2 CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$3" .)
}
build darwin arm64 "$bin/arm64"
build darwin amd64 "$bin/amd64"
lipo -create -output "$bin/mac-x64/varu-installer" "$bin/arm64" "$bin/amd64"
cp "$bin/mac-x64/varu-installer" "$bin/mac-arm64/" # identical in both halves of the universal app
rm "$bin/arm64" "$bin/amd64"
build windows amd64 "$bin/win-x64/varu-installer.exe"
build linux amd64 "$bin/linux-x64/varu-installer"
build linux arm64 "$bin/linux-arm64/varu-installer"

cd "$here"
npm ci --no-audit --no-fund
rm -rf ../build/desktop
case $version in v[0-9]*) semver=${version#v} ;; *) semver=0.0.0-dev ;; esac
npx electron-builder --mac --win --linux --publish never -c.extraMetadata.version="$semver"
cp ../build/desktop/Varu-macOS.dmg ../build/desktop/Varu-Setup-Windows.exe ../build/desktop/Varu-Windows.exe \
  ../build/desktop/Varu-Linux-*.AppImage ../build/desktop/varu_*.deb "$out/"
ls -l "$out"
