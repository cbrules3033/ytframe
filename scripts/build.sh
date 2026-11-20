#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION=$(sed -n 's/^const appVersion = "\(.*\)"/\1/p' cmd/ytframe/settings.go)
DIST=dist/YTFrame-win64

if [[ ! -f bundle/mpv.exe || ! -f bundle/yt-dlp.exe ]]; then
  echo "bundle/mpv.exe and bundle/yt-dlp.exe are required (see SOURCES.txt)" >&2
  exit 1
fi

rm -rf "$DIST" dist/YTFrame-win64.zip dist/YTFrame-win64.sha256
mkdir -p "$DIST"

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -buildvcs=false -trimpath \
  -ldflags "-H windowsgui -s -w" \
  -o "$DIST/YTFrame.exe" ./cmd/ytframe

cp bundle/mpv.exe bundle/yt-dlp.exe "$DIST/"
cp README.txt LICENSES.txt SOURCES.txt LICENSE LICENSE.GPL-2.0 "$DIST/"

(
  cd dist
  7z a -tzip YTFrame-win64.zip YTFrame-win64 >/dev/null
  sha256sum YTFrame-win64.zip > YTFrame-win64.sha256
)

echo "built $DIST/YTFrame.exe (v$VERSION)"
ls -lh dist/YTFrame-win64.zip dist/YTFrame-win64.sha256
