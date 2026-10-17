#!/bin/sh
# Checks the nginx front: the certificate script and `nginx -t` on the config,
# with the image docker-compose.yml pins.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

sh "$ROOT/scripts/gen-self-signed-cert.sh" "$WORK/certs" pentaract.lan 192.168.1.10
openssl x509 -in "$WORK/certs/fullchain.pem" -noout -ext subjectAltName \
  | grep -q "DNS:pentaract.lan, IP Address:192.168.1.10"
# Valid for 100 years: still valid 99 years from now.
openssl x509 -in "$WORK/certs/fullchain.pem" -noout -checkend $((99 * 365 * 24 * 3600)) >/dev/null

# Without a terminal (CI, scripts) it does not wait for an answer.
sh "$ROOT/scripts/gen-self-signed-cert.sh" "$WORK/no-tty" </dev/null
openssl x509 -in "$WORK/no-tty/fullchain.pem" -noout -ext subjectAltName | grep -q "DNS:localhost"

# A second run keeps the existing certificate.
before="$(openssl x509 -in "$WORK/certs/fullchain.pem" -noout -fingerprint)"
sh "$ROOT/scripts/gen-self-signed-cert.sh" "$WORK/certs" other.lan
after="$(openssl x509 -in "$WORK/certs/fullchain.pem" -noout -fingerprint)"
[ "$before" = "$after" ]

# Same official image and tag as docker-compose.yml, pulled from Docker Hub:
# ECR Public refuses anonymous pulls from GitHub runners ("Data limit exceeded").
TAG="$(sed -n 's/^ *image: .*nginx:\(.*\)$/\1/p' "$ROOT/docker-compose.yml")"
IMAGE="docker.io/library/nginx:$TAG"
docker run --rm --add-host pentaract:127.0.0.1 \
  -v "$ROOT/nginx/pentaract.conf:/etc/nginx/conf.d/default.conf:ro" \
  -v "$WORK/certs:/etc/nginx/certs:ro" \
  "$IMAGE" nginx -t

echo "nginx config OK"
