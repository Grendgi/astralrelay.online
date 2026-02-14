#!/bin/sh
set -e
CERT_DIR="/etc/xray"
if [ ! -f "$CERT_DIR/cert.pem" ] || [ ! -f "$CERT_DIR/key.pem" ]; then
  echo "Generating self-signed certs for Xray..."
  mkdir -p "$CERT_DIR"
  openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
    -keyout "$CERT_DIR/key.pem" -out "$CERT_DIR/cert.pem" \
    -subj "/CN=xray.local/O=Messenger"
fi
exec xray run -config /etc/xray/config.json
