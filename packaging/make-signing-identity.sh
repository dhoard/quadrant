#!/usr/bin/env bash
#
# Create the self-signed code-signing identity Quadrant can be signed with,
# once. Nothing here costs money, leaves the machine, or involves Apple.
#
# Why it is worth doing: macOS ties the Accessibility grant to the application's
# *signature*, and the signature build.sh makes by default is ad-hoc — a hash of
# the code itself. Every rebuild is therefore a different application to macOS,
# the stored grant stops matching ("Failed to match existing code requirement"),
# and the permission has to be given again. A self-signed certificate gives the
# bundle a requirement that names the certificate rather than the code:
#
#   ad-hoc:      designated => cdhash H"e4d30f5d…"                  changes every build
#   self-signed: designated => identifier "com.quadrant.app" and certificate leaf = H"d0bb6ab1…"
#
# The second line is what TCC re-checks, and it is the same after every rebuild,
# so the grant survives one. A Developer ID does the same thing and is what you
# would use to ship the app to other people; this is the free local equivalent
# for building it for yourself.
#
# What it creates: a private key and a self-signed certificate with the
# code-signing extended key usage, in a keychain of your choosing (your login
# keychain by default). No trust setting is changed, because signing does not
# need one. Undo it with:
#   security delete-identity -c "Quadrant Local Signing"
#
# Usage: ./packaging/make-signing-identity.sh [--keychain PATH] [--name NAME]

set -euo pipefail

NAME="Quadrant Local Signing"
KEYCHAIN="$HOME/Library/Keychains/login.keychain-db"

while (( $# > 0 )); do
  case "$1" in
    --keychain)
      [[ -n "${2:-}" ]] || { echo "ERROR: --keychain needs a path." >&2; exit 1; }
      KEYCHAIN="$2"; shift 2 ;;
    --name)
      [[ -n "${2:-}" ]] || { echo "ERROR: --name needs a name." >&2; exit 1; }
      NAME="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *)
      echo "ERROR: Unknown option: $1" >&2; exit 1 ;;
  esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "ERROR: code signing is a macOS affair; run this on the Mac." >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "ERROR: openssl is needed to make the certificate (macOS ships one)." >&2
  exit 1
fi
if [[ ! -e "$KEYCHAIN" ]]; then
  echo "ERROR: no keychain at $KEYCHAIN" >&2
  exit 1
fi

# An identity of this name already in the keychain is the whole point of running
# this once: say so rather than making a second one.
if security find-identity -p codesigning "$KEYCHAIN" 2>/dev/null | grep -qF "$NAME"; then
  echo "Already there: $NAME in $KEYCHAIN"
  echo "Build with: QUADRANT_SIGN_IDENTITY=\"$NAME\" ./build.sh"
  exit 0
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/quadrant-identity.XXXXXX")"
trap 'rm -rf "$work"' EXIT

cat >"$work/openssl.cnf" <<EOF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $NAME
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
EOF

echo "Creating a self-signed code-signing certificate ($NAME)..."
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout "$work/key.pem" -out "$work/cert.pem" -config "$work/openssl.cnf" >/dev/null 2>&1
# Add the certificate as DER: `security add-certificates` takes a certificate
# file, and the private key is imported separately. A PKCS#12 would need the
# legacy algorithms the Security framework insists on, which recent OpenSSL no
# longer writes by default -- two imports are simpler and have no such trap.
openssl x509 -in "$work/cert.pem" -outform DER -out "$work/cert.der"

# -T /usr/bin/codesign: the key is usable by codesign and by nothing else. The
# first build may still ask for permission to use it -- "codesign wants to use
# the key in your keychain" -- and Always Allow that once.
security import "$work/key.pem" -k "$KEYCHAIN" -T /usr/bin/codesign >/dev/null
# -k before the file: with the keychain named last, `security` reads "-k" as a
# certificate path and fails with "Error reading file -k".
security add-certificates -k "$KEYCHAIN" "$work/cert.der"

echo
 echo "Created: $NAME in $KEYCHAIN"
security find-identity -p codesigning "$KEYCHAIN" | grep -F "$NAME" || true
echo "(CSSMERR_TP_NOT_TRUSTED above is expected: this certificate is not Apple's,"
echo " which is the point. Signing does not need a trust setting, and none was changed.)"
echo
echo "Build with it:"
echo "  QUADRANT_SIGN_IDENTITY=\"$NAME\" ./build.sh"
echo
echo "Then grant Quadrant Accessibility once. That grant is keyed to this"
echo "certificate, so later rebuilds will not ask for it again."
