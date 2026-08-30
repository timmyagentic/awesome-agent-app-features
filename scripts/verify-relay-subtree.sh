#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
temporary_root=$(mktemp -d "${TMPDIR:-/tmp}/agent-app-features-relay.XXXXXX")
relay_root="$temporary_root/relay"

cleanup() {
  rm -rf "$temporary_root"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$relay_root"
(
  cd "$repository_root/relay/cloudflare"
  tar --exclude node_modules --exclude .wrangler -cf - .
) | (
  cd "$relay_root"
  tar -xf -
)

sh "$relay_root/verify.sh"
