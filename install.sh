#!/usr/bin/env bash
#
# Install the bundle build.sh produced.
#
# A build is ad-hoc signed by default, and macOS ties an Accessibility grant to
# the code signature. Copying a new build over the old one therefore leaves a
# grant that no longer matches. This script stops the running app, copies the
# new bundle into place, resets the stale Accessibility entry for an ad-hoc
# build, relaunches the app, and opens the pane where the grant is given.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

LABEL="com.quadrant.agent"
FALLBACK_BUNDLE_ID="com.quadrant.app"
SOURCE="$ROOT/dist/.build/Quadrant.app"
DEST_DIR="/Applications"
KEEP_ACCESSIBILITY=false
RELAUNCH=true

die() {
  echo "ERROR: $*" >&2
  exit 1
}

note() {
  echo "note: $*" >&2
}

usage() {
  cat <<'EOF'
Usage: ./install.sh [options]

Installs dist/.build/Quadrant.app. The running app is stopped first. An ad-hoc
build has its Accessibility grant reset because its signature changed.

Options:
  --dest DIR              Install into DIR/Quadrant.app (default: /Applications)
  --keep-accessibility    Do not reset the Accessibility grant
  --no-relaunch           Copy only; do not start Quadrant afterwards
  -h, --help              Show this help

Run ./build.sh first.
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --dest)
      [[ -n "${2:-}" ]] || die "--dest needs a directory"
      DEST_DIR="$2"
      shift 2
      ;;
    --keep-accessibility)
      KEEP_ACCESSIBILITY=true
      shift
      ;;
    --no-relaunch)
      RELAUNCH=false
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1 (try --help)"
      ;;
  esac
done

[[ "$(uname -s)" == "Darwin" ]] || die "this installer is for macOS"
[[ -d "$SOURCE" ]] || die "no build at $SOURCE; run ./build.sh first"
[[ -x "$SOURCE/Contents/MacOS/Quadrant" ]] || die "$SOURCE has no executable"

DEST_DIR="${DEST_DIR%/}"
[[ -n "$DEST_DIR" && "$DEST_DIR" != "/" ]] || die "refusing to install into ${DEST_DIR:-/}"
DEST="$DEST_DIR/Quadrant.app"

if command -v plutil >/dev/null 2>&1; then
  plutil -lint "$SOURCE/Contents/Info.plist" >/dev/null || die "invalid Info.plist in $SOURCE"
fi
if command -v codesign >/dev/null 2>&1; then
  codesign --verify --strict "$SOURCE" >/dev/null 2>&1 ||
    die "$SOURCE has an invalid signature; rerun ./build.sh"
fi

if [[ ! -d "$DEST_DIR" ]]; then
  mkdir -p "$DEST_DIR" 2>/dev/null || die "cannot create $DEST_DIR"
fi
[[ -w "$DEST_DIR" ]] ||
  die "cannot write to $DEST_DIR; use --dest \"$HOME/Applications\" or fix its permissions"

uid="$(id -u)"
agent_plist="$HOME/Library/LaunchAgents/$LABEL.plist"
agent_loaded=false
agent_matches=false
if launchctl print "gui/$uid/$LABEL" >/dev/null 2>&1; then
  agent_loaded=true
  if [[ -f "$agent_plist" ]] && command -v plutil >/dev/null 2>&1; then
    agent_program="$(plutil -extract ProgramArguments.0 raw -o - "$agent_plist" 2>/dev/null || true)"
    [[ "$agent_program" == "$DEST/Contents/MacOS/Quadrant" ]] && agent_matches=true
  fi
fi

if $agent_loaded; then
  echo "Stopping LaunchAgent $LABEL..."
  launchctl bootout "gui/$uid/$LABEL" >/dev/null 2>&1 ||
    die "could not stop LaunchAgent $LABEL; quit Quadrant and rerun"
fi

pids="$(pgrep -x Quadrant || true)"
if [[ -n "$pids" ]]; then
  echo "Stopping Quadrant (PIDs:$pids)..."
  # PIDs are digits; splitting on whitespace is safe here.
  # shellcheck disable=SC2086
  for pid in $pids; do
    kill -TERM "$pid" 2>/dev/null || true
  done
  still=""
  for ((i = 0; i < 50; i++)); do
    still=""
    # shellcheck disable=SC2086
    for pid in $pids; do
      if kill -0 "$pid" 2>/dev/null; then
        still="$still $pid"
      fi
    done
    [[ -z "$still" ]] && break
    sleep 0.1
  done
  [[ -z "$still" ]] || die "Quadrant did not stop (PIDs:$still); quit it and rerun"
fi

staging="$(mktemp -d "${TMPDIR:-/tmp}/quadrant-install.XXXXXX")"
trap 'rm -rf "$staging"' EXIT

if command -v ditto >/dev/null 2>&1; then
  ditto "$SOURCE" "$staging/Quadrant.app"
else
  cp -R "$SOURCE" "$staging/Quadrant.app"
fi
if command -v codesign >/dev/null 2>&1; then
  codesign --verify --strict "$staging/Quadrant.app" >/dev/null 2>&1 ||
    die "the copied bundle has an invalid signature"
fi

case "$DEST" in
  */Quadrant.app) ;;
  *) die "refusing to remove unexpected destination: $DEST" ;;
esac

echo "Installing $DEST..."
rm -rf "$DEST"
mv "$staging/Quadrant.app" "$DEST"

if command -v codesign >/dev/null 2>&1; then
  codesign --verify --strict "$DEST" >/dev/null 2>&1 ||
    die "the installed bundle has an invalid signature"
fi

bundle_id="$FALLBACK_BUNDLE_ID"
if command -v plutil >/dev/null 2>&1; then
  extracted="$(plutil -extract CFBundleIdentifier raw -o - "$DEST/Contents/Info.plist" 2>/dev/null || true)"
  [[ -n "$extracted" ]] && bundle_id="$extracted"
fi

if ! "$DEST/Contents/MacOS/Quadrant" -config "$HOME/.quadrant/configuration.json" -check-config >/dev/null 2>&1; then
  note "the installed binary rejected ~/.quadrant/configuration.json; fix it before launching"
fi

adhoc=false
if command -v codesign >/dev/null 2>&1; then
  signature="$(codesign -dv --verbose=4 "$DEST" 2>&1 || true)"
  if [[ "$signature" == *"Signature=adhoc"* || "$signature" == *"(adhoc)"* ]]; then
    adhoc=true
  fi
else
  adhoc=true
fi

reset_accessibility=false
if $KEEP_ACCESSIBILITY; then
  echo "Keeping the existing Accessibility grant."
elif $adhoc; then
  echo "Ad-hoc signature: resetting Accessibility for $bundle_id..."
  if command -v tccutil >/dev/null 2>&1; then
    tccutil reset Accessibility "$bundle_id" ||
      note "could not reset Accessibility; reset it in System Settings"
  else
    note "tccutil is missing; reset Accessibility in System Settings"
  fi
  reset_accessibility=true
else
  echo "Stable signature: keeping the existing Accessibility grant."
fi

echo
echo "Installed: $DEST"

if $RELAUNCH; then
  if $agent_loaded && $agent_matches; then
    echo "Starting LaunchAgent $LABEL..."
    if ! launchctl bootstrap "gui/$uid" "$agent_plist" >/dev/null 2>&1; then
      launchctl kickstart -k "gui/$uid/$LABEL" >/dev/null 2>&1 || open "$DEST"
    fi
  else
    if $agent_loaded; then
      note "the loaded LaunchAgent pointed at another path and was stopped; opening $DEST directly"
    fi
    echo "Opening $DEST..."
    open "$DEST"
  fi
  sleep 1
  if ! pgrep -x Quadrant >/dev/null 2>&1; then
    note "Quadrant is not running yet; open $DEST manually"
  fi
elif $agent_loaded; then
  note "LaunchAgent $LABEL was stopped; start it with: launchctl bootstrap gui/$uid $agent_plist"
fi

if $reset_accessibility; then
  echo
  echo "Accessibility was reset. In System Settings, add or switch on Quadrant:"
  echo "  $DEST"
  echo "The app polls every 2 seconds and registers the shortcuts as soon as the grant is on."
  if $RELAUNCH; then
    open "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility" >/dev/null 2>&1 || true
  fi
fi

if $agent_loaded && $agent_matches; then
  echo
  echo "Log: $HOME/Library/Logs/quadrant/quadrant.log"
fi
