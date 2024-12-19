# SPDX-License-Identifier: ice License 1.0

# Generate a new ecdsa key/cert for local development of WebTransport
HOST="localhost"
CRT="$HOST.crt"
KEY="$HOST.key"
# Install the system certificate if it's not already
go get filippo.io/mkcert
go run filippo.io/mkcert -ecdsa -install
cp $(go run filippo.io/mkcert -CAROOT)/rootCA.pem ca.pem

# Generate a new certificate for localhost
NETWORK_IP=$(ifconfig | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*' | grep -v '127.0.0.1' | tail -1)
go run filippo.io/mkcert -ecdsa -days 13 -cert-file "$CRT" -key-file "$KEY" localhost $(hostname) 127.0.0.1 ::1 $NETWORK_IP

# Compute the sha256 fingerprint of the certificate for WebTransport
rm fingerprint.base64 || true
openssl x509 -in localhost.crt | openssl dgst -sha256 -binary | base64 > fingerprint.base64