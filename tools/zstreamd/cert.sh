#!/bin/sh
set -eu

name=${CERT_NAME:-zored-stream}
keychain=${KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}

if security find-certificate -c "$name" "$keychain" >/dev/null 2>&1; then
	echo "$name is already in $keychain"
	exit 0
fi

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
cat >"$dir/cert.cnf" <<EOF
[req]
distinguished_name = dn
prompt = no
x509_extensions = ext
[dn]
CN = $name
[ext]
basicConstraints = critical, CA:false
keyUsage = critical, digitalSignature
extendedKeyUsage = critical, codeSigning
EOF
/usr/bin/openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -config "$dir/cert.cnf" \
	-keyout "$dir/key.pem" -out "$dir/cert.pem"
/usr/bin/openssl pkcs12 -export -inkey "$dir/key.pem" -in "$dir/cert.pem" -name "$name" \
	-passout pass:import -out "$dir/identity.p12"
security import "$dir/identity.p12" -k "$keychain" -P import -T /usr/bin/codesign
if [ -z "${NO_TRUST:-}" ]; then
	security add-trusted-cert -r trustAsRoot -p codeSign -k "$keychain" "$dir/cert.pem"
fi
security find-identity -p codesigning "$keychain"
