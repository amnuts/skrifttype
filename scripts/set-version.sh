#!/usr/bin/env bash
# Rewrites every "0.0.0" placeholder in the build metadata to the version passed in $1.
# Called by .github/workflows/release.yml before running `wails3 build`.

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "usage: $0 <version>" >&2
  exit 1
fi

VERSION="$1"

# MSIX wants exactly four dotted components (X.Y.Z.0) and no pre-release suffix.
MSIX_VERSION="${VERSION%%-*}"
while [ "$(echo "$MSIX_VERSION" | awk -F. '{print NF}')" -lt 4 ]; do
  MSIX_VERSION="${MSIX_VERSION}.0"
done

replace() {
  local file="$1" pattern="$2" replacement="$3"
  if [ ! -f "$file" ]; then
    echo "skip (missing): $file"
    return
  fi
  local tmp
  tmp=$(mktemp)
  sed -e "s|${pattern}|${replacement}|g" "$file" > "$tmp"
  mv "$tmp" "$file"
  echo "updated: $file"
}

replace "wails.yaml"                              'version: 0\.0\.0 #'          "version: $VERSION #"
replace "build/darwin/Info.plist"             '<string>0\.0\.0</string>'     "<string>$VERSION</string>"
replace "build/darwin/Info.dev.plist"         '<string>0\.0\.0</string>'     "<string>$VERSION</string>"
replace "build/windows/info.json"             '"0\.0\.0"'                     "\"$VERSION\""
replace "build/windows/font-helper.info.json" '"0\.0\.0"'                     "\"$VERSION\""
replace "build/windows/msix/app_manifest.xml" 'Version="0\.0\.0\.0"'         "Version=\"$MSIX_VERSION\""
replace "build/windows/msix/template.xml"     'Version="0\.0\.0\.0"'         "Version=\"$MSIX_VERSION\""
replace "build/windows/nsis/wails_tools.nsh"  'INFO_PRODUCTVERSION "0\.0\.0"' "INFO_PRODUCTVERSION \"$VERSION\""

echo "Version set to $VERSION (MSIX: $MSIX_VERSION)"
