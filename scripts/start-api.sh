#!/usr/bin/env bash
# Start the DeepTrols API server
set -a
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../.env"
set +a
exec "$SCRIPT_DIR/../bin/api.exe"
