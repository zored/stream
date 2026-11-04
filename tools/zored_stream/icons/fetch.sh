#!/bin/sh
set -eu
V=1.11.3
cd "$(dirname "$0")"
for f in *.svg; do curl -fsS -o "$f" "https://cdn.jsdelivr.net/npm/bootstrap-icons@$V/icons/$f"; done
curl -fsS -o LICENSE "https://cdn.jsdelivr.net/npm/bootstrap-icons@$V/LICENSE"
