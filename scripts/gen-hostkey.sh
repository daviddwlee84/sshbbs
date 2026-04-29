#!/usr/bin/env bash
set -euo pipefail

OUT="${1:-.ssh/host_ed25519}"
mkdir -p "$(dirname "$OUT")"

if [ -f "$OUT" ]; then
  echo "host key already exists at $OUT" >&2
  exit 0
fi

ssh-keygen -t ed25519 -N "" -C "sshbbs-host" -f "$OUT"
echo "generated host key at $OUT"
