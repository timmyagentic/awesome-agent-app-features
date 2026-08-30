#!/bin/sh
set -eu

relay_root=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
cd "$relay_root"
npm ci --ignore-scripts
npm test
npm run check
npm run typecheck
npm run types:check
npm run validate:worker
npm audit --audit-level=high
