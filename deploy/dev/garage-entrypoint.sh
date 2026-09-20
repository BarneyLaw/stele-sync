#!/bin/sh
# Same idempotent layout/key/bucket bootstrap as phase 1 garage-dev.sh.
set -eu
/bin/busybox mkdir -p /tmp
/bin/busybox rm -f /tmp/garage-ready
/garage server &
server_pid=$!
trap 'kill -TERM "$server_pid" 2>/dev/null || true; wait "$server_pid" || true' EXIT
trap 'exit 0' INT TERM
attempt=0
until /garage status >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    echo 'Garage did not become ready' >&2
    exit 1
  fi
  /bin/busybox sleep 1
done
if /garage layout show 2>/dev/null | /bin/busybox grep -qiE 'no nodes|current cluster layout version: 0'; then
  node=$(/garage node id -q | /bin/busybox cut -d@ -f1)
  /garage layout assign -z dev -c 1G "$node"
  /garage layout apply --version 1
fi
for bucket in obsync obsync-sync; do
  /garage bucket info "$bucket" >/dev/null 2>&1 || /garage bucket create "$bucket"
  # Suppress key creation output: it includes the generated secret.
  /garage key info "$bucket-dev" >/dev/null 2>&1 || /garage key create "$bucket-dev" >/dev/null
  /garage bucket allow --read --write --owner "$bucket" --key "$bucket-dev"
done
/garage bucket website --allow obsync
/bin/busybox touch /tmp/garage-ready
wait "$server_pid"
