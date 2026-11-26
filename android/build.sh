#!/bin/sh
# Builds Varu.apk with the Android SDK's own tools: no Gradle, no libraries.
#   sh android/build.sh v1.2.3 dist
# Needs Go, a JDK (11+) and an Android SDK with a platform and build-tools
# (ANDROID_HOME, or ~/Library/Android/sdk).
#
# Signing: set ANDROID_KEYSTORE (path to a keystore) and ANDROID_KEYSTORE_PASS
# for releases. Without them the debug key in ~/.android is used; Android
# refuses to update an app signed with a different key, so releases must
# always use the same keystore.
set -eu
version=${1:-dev}
mkdir -p "${2:-dist}"
out=$(cd "${2:-dist}" && pwd)
repo=$(cd "$(dirname "$0")/.." && pwd)
sdk=${ANDROID_HOME:-$HOME/Library/Android/sdk}
platform=$(ls -d "$sdk"/platforms/android-* | sort -V | tail -1)
tools=$(ls -d "$sdk"/build-tools/* | sort -V | tail -1)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# v1.2.3 -> 10203; anything else -> 1
code=$(echo "$version" | awk -F'[v.]' 'NF==4 && $2$3$4 ~ /^[0-9]+$/ {print $2*10000+$3*100+$4; exit} {print 1}')

# 1. The Go programs, named lib*.so so Android unpacks them where they may run.
# 64-bit ARM only (Go needs cgo for 32-bit Android); that is nearly every phone sold since 2017.
libs="$work/apk/lib/arm64-v8a"
mkdir -p "$libs"
for program in installer launcher; do
  (cd "$repo/$program" && GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$libs/libvaru$program.so" .)
done

# 2. Resources, manifest, Java, dex.
"$tools/aapt2" compile --dir "$repo/android/res" -o "$work/res.zip"
"$tools/aapt2" link -o "$work/app.apk" -I "$platform/android.jar" \
  --manifest "$repo/android/AndroidManifest.xml" --version-code "$code" --version-name "${version#v}" "$work/res.zip"
# find -exec rather than $(find), so a folder name with a space still works.
find "$repo/android/src" -name '*.java' -exec \
  javac -nowarn --release 11 -classpath "$platform/android.jar" -d "$work/classes" {} +
find "$work/classes" -name '*.class' -exec \
  "$tools/d8" --release --min-api 24 --lib "$platform/android.jar" --output "$work/apk" {} +

# 3. Assemble, align, sign.
(cd "$work/apk" && zip -qr "$work/app.apk" classes.dex lib)
"$tools/zipalign" -f 4 "$work/app.apk" "$work/aligned.apk"
if [ -n "${ANDROID_KEYSTORE:-}" ]; then
  ks=$ANDROID_KEYSTORE pass=$ANDROID_KEYSTORE_PASS
else
  ks=$HOME/.android/debug.keystore pass=android
  if [ ! -f "$ks" ]; then
    mkdir -p "$HOME/.android"
    keytool -genkeypair -keystore "$ks" -storepass android -keypass android -alias androiddebugkey \
      -dname "CN=Android Debug,O=Android,C=US" -keyalg RSA -validity 10000 >/dev/null
  fi
  echo "warning: signed with a debug key; set ANDROID_KEYSTORE for a release" >&2
fi
"$tools/apksigner" sign --v4-signing-enabled false --ks "$ks" --ks-pass "pass:$pass" --out "$out/Varu.apk" "$work/aligned.apk"
echo "wrote $out/Varu.apk"
