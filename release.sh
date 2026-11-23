#!/bin/sh
# Builds every release file on this Mac and publishes them as a GitHub
# release; no GitHub Actions involved.
#
#   sh release.sh v0.1.2            build, tag and publish
#   sh release.sh v0.1.2 --dry-run  build only; the files end up in dist/
#
# Needs macOS (for the .app), Go, Python 3.11+, the pmtiles command, a JDK,
# an Android SDK, gh logged in, and the Android release key: the keystore
# at ~/.android/varu-release.keystore and its password in the login
# Keychain under "Varu Android release keystore".
set -eu
version=${1:?usage: sh release.sh vX.Y.Z [--dry-run]}
mode=${2:-}
regions="ee" # one id per country offered in the installers
cd "$(dirname "$0")"

fail() { echo "release: $*" >&2; exit 1; }
case "$version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) fail "version must look like v1.2.3" ;; esac
case "$mode" in "" | --dry-run) ;; *) fail "unknown option $mode" ;; esac

if [ "$mode" != --dry-run ]; then
  # Publish exactly what is on GitHub's main, so the release matches the code.
  git fetch -q origin
  [ -z "$(git status --porcelain)" ] || fail "commit or stash your changes first"
  [ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || fail "check out an up-to-date main first"
  if git ls-remote --exit-code --tags origin "refs/tags/$version" >/dev/null; then
    fail "$version already exists on GitHub"
  fi
fi

keystore=$HOME/.android/varu-release.keystore
[ -f "$keystore" ] || fail "missing $keystore"
pass=$(security find-generic-password -a "$USER" -s "Varu Android release keystore" -w) ||
  fail "the release key's password is not in the Keychain"
command -v pmtiles >/dev/null || fail "install pmtiles first: brew install pmtiles"
command -v lipo >/dev/null || fail "run this on a Mac (the macOS app needs lipo)"

make test

rm -rf dist
for region in $regions; do
  # Fresh map and places every release; the app, launchers and readers are reused.
  rm -rf "build/$region/maps" "build/$region/pois"
  make release-content REGION="$region"
done
make installer-windows installer-macos VERSION="$version"
ANDROID_KEYSTORE=$keystore ANDROID_KEYSTORE_PASS=$pass make android VERSION="$version"
rm -rf "dist/Varu Installer.app" # shipped zipped as Varu-Installer-macOS.zip

if [ "$mode" = --dry-run ]; then
  echo
  echo "Dry run done. Release files in dist/:"
  ls -l dist
  exit 0
fi

git tag -a "$version" -m "Varu ${version#v}"
git push -q origin "$version"
gh release create "$version" dist/* --verify-tag --title "Varu $version" --notes \
  "Download the installer for your system: Varu-Installer.exe (Windows), Varu-Installer-macOS.zip (macOS) or Varu.apk (Android). The other files are what the installers download."
echo "Published $version"
