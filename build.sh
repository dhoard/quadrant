#!/usr/bin/env bash
set -euo pipefail

# This project targets Apple Silicon. A Rosetta-translated shell makes universal
# tools such as git and xcrun choose their x86_64 slices, while the installed
# Command Line Tools libraries are arm64-only. Re-exec under arm64 so a plain
# ./build.sh invocation works without extra flags or environment variables.
if [[ "$(uname -s)" == "Darwin" && "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" == "1" ]]; then
  echo "build.sh: current shell is running under Rosetta; re-executing under arm64" >&2
  exec arch -arm64 /bin/bash "${BASH_SOURCE[0]}" "$@"
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: Go 1.26 or later must be installed and on PATH." >&2
  exit 1
fi

VERSION="$(go env GOVERSION)"
if [[ ! "$VERSION" =~ ^go1\.([0-9]+) ]]; then
  echo "ERROR: Cannot determine Go version: $VERSION" >&2
  exit 1
fi
if (( BASH_REMATCH[1] < 26 )); then
  echo "ERROR: Go 1.26 or later is required (found $VERSION)." >&2
  exit 1
fi

echo "Using $(go version)"
echo "Resolving Go modules (network required only on first build)..."
GOTOOLCHAIN=local go mod tidy

echo "Running tests..."
CGO_ENABLED=0 GOTOOLCHAIN=local go test ./...

# The About panel's version. A source build outside a git repository says so
# rather than pretending to be a release.
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
# CFBundleShortVersionString is digits and dots, so a description such as
# "v1.2.0-3-gabc1234-dirty" becomes "1.2.0" and a bare commit hash becomes
# 0.0.0. The About panel still shows the untrimmed description.
BUNDLE_VERSION="$(printf '%s' "$VERSION" | sed -n 's/^v\{0,1\}\([0-9][0-9.]*\).*$/\1/p')"
[[ -n "$BUNDLE_VERSION" ]] || BUNDLE_VERSION="0.0.0"

APP="$ROOT/dist/.build/Quadrant.app"
# The executable is named for the application, not for the module: it is what
# macOS shows when no bundle name applies, and what Activity Monitor lists.
EXEC="$APP/Contents/MacOS/Quadrant"

# The name packaging/make-signing-identity.sh gives the free, local code-signing
# identity. build.sh only ever mentions it; it is used when the environment asks
# for it by name.
LOCAL_IDENTITY="Quadrant Local Signing"

# The bundle is built into a HIDDEN directory, and that is deliberate: Spotlight
# does not index hidden items, and Spotlight's application index is where the
# macOS launcher gets its icons. A bundle built in the open is a second
# "Quadrant" in that list, beside the installed one; hidden, it is a build
# artefact and nothing else. Opening it from the Finder registers it with
# LaunchServices all the same, so run the installed copy, not this one.

# dist/ holds this build's output and nothing else, so it is emptied first: an
# earlier layout left a bare `quadrant` executable next to the bundle, which is
# neither built nor read any more, and a stale 3 MB binary beside Quadrant.app
# reads like part of it. .gitkeep stays -- it is a tracked file, and it is what
# keeps the empty directory in a fresh clone. Anything you drop in here is build
# output to this script, and will not survive the next run. The hidden build
# directory goes with it, and is recreated below.
if [[ -d "$ROOT/dist" ]]; then
  echo "Clearing dist/ (keeping .gitkeep)..."
  find "$ROOT/dist" -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} +
fi

echo "Building macOS ARM64 application bundle ($VERSION)..."
mkdir -p "$APP/Contents/MacOS"
sed -e "s|__VERSION__|$BUNDLE_VERSION|g" "$ROOT/packaging/Info.plist" > "$APP/Contents/Info.plist"

CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=local \
  go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$EXEC" ./cmd/quadrant
chmod +x "$EXEC"

# The icon, before the bundle is signed: the mark the menu bar draws, at every
# size an .iconset needs, converted by iconutil(1), Apple's own tool. Drawing it
# needs AppKit and converting it needs iconutil, so a bundle built where neither
# exists has no icon and the Finder shows the generic one -- which is why the
# step is skipped loudly rather than failing the build.
if [[ "$(uname -s)" == "Darwin" ]] && command -v iconutil >/dev/null 2>&1; then
  echo "Drawing the application icon..."
  iconset="$(mktemp -d "${TMPDIR:-/tmp}/quadrant-icon.XXXXXX")/AppIcon.iconset"
  CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/icongen -out "$iconset" >/dev/null
  mkdir -p "$APP/Contents/Resources"
  iconutil --convert icns --output "$APP/Contents/Resources/AppIcon.icns" "$iconset"
  rm -rf "$(dirname "$iconset")"
else
  echo "note: no application icon: drawing one needs macOS and iconutil(1)." >&2
fi

if command -v plutil >/dev/null 2>&1; then
  plutil -lint "$APP/Contents/Info.plist" >/dev/null
fi

if command -v codesign >/dev/null 2>&1; then
  # The Accessibility grant follows the code signature: macOS identifies the
  # application by it, and the grant is dropped when the signature changes. An
  # ad-hoc signature ("-", the default) is a hash of the code, so every rebuild
  # is a different application and the grant has to be given again. Set
  # QUADRANT_SIGN_IDENTITY to an identity that keeps its requirement stable --
  # a "Developer ID Application: …" certificate, or the free self-signed one
  # packaging/make-signing-identity.sh creates -- and the grant survives
  # rebuilds. QUADRANT_SIGN_KEYCHAIN names the keychain holding it, for an
  # identity that is not in the login keychain.
  IDENTITY="${QUADRANT_SIGN_IDENTITY:--}"
  xattr -cr "$APP" # a stray extended attribute makes codesign refuse the bundle
  sign=(--force --sign "$IDENTITY")
  if [[ -n "${QUADRANT_SIGN_KEYCHAIN:-}" ]]; then
    sign+=(--keychain "$QUADRANT_SIGN_KEYCHAIN")
  fi
  codesign "${sign[@]}" "$APP"
  codesign --verify --strict "$APP"
  if [[ "$IDENTITY" == "-" ]]; then
    echo "Signed:  ad-hoc (QUADRANT_SIGN_IDENTITY is unset)"
    # Worth saying out loud, because the bill for ad-hoc signing arrives later,
    # as a permission dialog after every rebuild.
    if security find-identity -p codesigning 2>/dev/null | grep -qF "$LOCAL_IDENTITY"; then
      echo "note:    the local identity \"$LOCAL_IDENTITY\" is in your keychain and would keep the"
      echo "         Accessibility grant across rebuilds: QUADRANT_SIGN_IDENTITY=\"$LOCAL_IDENTITY\" ./build.sh"
    else
      echo "note:    a free self-signed identity would keep the Accessibility grant across"
      echo "         rebuilds: ./packaging/make-signing-identity.sh"
    fi
  else
    echo "Signed:  $IDENTITY"
  fi
else
  echo "note: codesign not found, so the bundle is unsigned. macOS will treat every" >&2
  echo "      rebuild as a new application and any Accessibility grant will be lost." >&2
fi

echo "Built: $APP"
echo "Install: ./install.sh"
echo "Run:   open $APP"
echo "       or in the foreground: $EXEC"
