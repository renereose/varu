#!/bin/sh
# Publishes a built web app site (web/build.sh) on GitHub Pages:
#
#   sh web/publish.sh build/web-site v0.1.12
#
# The gh-pages branch is replaced by a single commit holding just the
# site, so old maps do not pile up in the repository. Turns Pages on for
# that branch the first time. GitHub then serves it at
# https://<owner>.github.io/<repo>/.
set -eu
site=${1:?usage: sh web/publish.sh <site> <version>}
version=${2:?usage: sh web/publish.sh <site> <version>}
repo=$(cd "$(dirname "$0")/.." && pwd)
[ -f "$site/index.html" ] && [ -f "$site/sw.js" ] || { echo "publish: $site is not a built site" >&2; exit 1; }
remote=$(git -C "$repo" remote get-url origin)
name=$(git -C "$repo" config user.name)
email=$(git -C "$repo" config user.email)
slug=$(cd "$repo" && gh repo view --json nameWithOwner -q .nameWithOwner)

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp -R "$site"/. "$tmp"/
cd "$tmp"
git init -q -b gh-pages
git add -A
git -c user.name="$name" -c user.email="$email" commit -q -m "Web app $version"
git push -q -f "$remote" gh-pages

if ! gh api "repos/$slug/pages" >/dev/null 2>&1; then
  gh api -X POST "repos/$slug/pages" -f "source[branch]=gh-pages" -f "source[path]=/" >/dev/null
fi
echo "Published the web app: $(gh api "repos/$slug/pages" -q .html_url)"
