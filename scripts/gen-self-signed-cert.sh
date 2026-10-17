#!/bin/sh
# Creates a self-signed certificate for the nginx front, unless one exists.
#
# Usage: gen-self-signed-cert.sh <cert-dir> [extra hosts...]
# Extra hosts are DNS names or IPs (e.g. pentaract.lan 192.168.1.10) added
# to the certificate next to localhost, 127.0.0.1 and this machine's name.
# With no extra hosts and a terminal attached, it asks for the FQDN.
# To use a real certificate, put fullchain.pem and privkey.pem in <cert-dir>.
set -eu

CERT_DIR="${1:?usage: $0 <cert-dir> [extra hosts...]}"
shift

# 100 years: a self-signed certificate is accepted once in the browser and
# should never have to be renewed.
VALIDITY_DAYS=36500

if [ -f "$CERT_DIR/fullchain.pem" ] && [ -f "$CERT_DIR/privkey.pem" ]; then
  echo "Certificate already present in $CERT_DIR, keeping it."
  exit 0
fi

if [ "$#" -eq 0 ] && [ -t 0 ]; then
  printf 'FQDN or IP you open Pentaract with (several separated by spaces, Enter for localhost only): '
  read -r answer || answer=""
  # shellcheck disable=SC2086 # split the answer into one argument per host
  set -- $answer
fi

san="DNS:localhost,IP:127.0.0.1"
for host in "$(hostname)" "$@"; do
  [ -n "$host" ] || continue
  if echo "$host" | grep -Eq '^[0-9]+(\.[0-9]+){3}$|:'; then
    san="$san,IP:$host"
  else
    san="$san,DNS:$host"
  fi
done

mkdir -p "$CERT_DIR"
openssl req -x509 -newkey rsa:2048 -nodes -days "$VALIDITY_DAYS" \
  -keyout "$CERT_DIR/privkey.pem" -out "$CERT_DIR/fullchain.pem" \
  -subj "/CN=pentaract" -addext "subjectAltName=$san" 2>/dev/null
chmod 600 "$CERT_DIR/privkey.pem"

echo "Self-signed certificate written to $CERT_DIR ($san)."
