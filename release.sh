#!/bin/sh
# Builds every release file on this Mac and publishes them as a GitHub
# release, and the web app on GitHub Pages (web/publish.sh); no workflows
# of our own.
#
#   sh release.sh v0.1.2              build, tag and publish
#   sh release.sh v0.1.2 --dry-run    build only; the files end up in dist/
#   sh release.sh v0.1.2 --keep-data  reuse the last build's map, roads,
#                                     addresses and places instead of
#                                     downloading them again (an app-only
#                                     release; the files are still uploaded,
#                                     as the installer takes them from here)
#
# Needs macOS (for the Mac app), Go, Node, Python 3.11+, the pmtiles command, a JDK,
# an Android SDK, gh logged in, and the Android release key: the keystore
# at ~/.android/varu-release.keystore and its password in the login
# Keychain under "Varu Android release keystore".
set -eu
fail() { echo "release: $*" >&2; exit 1; }
version=${1:?usage: sh release.sh vX.Y.Z [--dry-run] [--keep-data]}
shift
dry="" keep=""
for opt in "$@"; do
  case "$opt" in --dry-run) dry=1 ;; --keep-data) keep=1 ;; *) fail "unknown option $opt" ;; esac
done
regions="ee" # one id per country offered in the installers
cd "$(dirname "$0")"

case "$version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) fail "version must look like v1.2.3" ;; esac

if [ -z "$dry" ]; then
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
# The desktop apps first: regions.json lists them for USB sticks.
make desktop VERSION="$version"
for region in $regions; do
  # Fresh map and places every release, unless --keep-data keeps the last
  # build's (what is missing is still built); the app is always rebuilt.
  [ -n "$keep" ] || rm -rf "build/$region/maps" "build/$region/pois"
  make release-content REGION="$region"
done
# The library's hand-picked articles (viewer/topics.json) must exist in the
# books just indexed, which sit in the release's download cache.
(cd launcher && VARU_ZIM_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/varu/zim" go test -count=1 -run TestTopics .)
ANDROID_KEYSTORE=$keystore ANDROID_KEYSTORE_PASS=$pass make android VERSION="$version"
# The web app (for iPhones and browsers), from the first country's drive.
sh web/build.sh "build/${regions%% *}" build/web-site "$version"

if [ -n "$dry" ]; then
  echo
  echo "Dry run done. Release files in dist/:"
  ls -l dist
  exit 0
fi

git tag -a "$version" -m "Varu ${version#v}"
git push -q origin "$version"
gh release create "$version" dist/* --verify-tag --title "Varu $version" --notes \
"Download Varu for your system:
- macOS: Varu-macOS.dmg. The first time, macOS may say it cannot check Varu: open System Settings → Privacy & Security and click Open Anyway.
- Windows: Varu-Setup-Windows.exe. If Windows protects your PC: More info → Run anyway.
- Linux: varu_${version#v}_amd64.deb (Debian, Ubuntu), or Varu-Linux-x86_64.AppImage. The arm64 files are for a Raspberry Pi 4 or 5.
- Android: Varu.apk.

The other files are what the app downloads."
echo "Published $version"
sh web/publish.sh build/web-site "$version"
