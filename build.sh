#!/usr/bin/env bash
set -euo pipefail

echo "Building glm-quota.so using Docker (Linux amd64)..."
docker run --rm -v "$(pwd):/src" -w /src golang:1.24-bookworm bash -c \
  "CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -buildmode=c-shared -o glm-quota.so ."
echo "Build complete: glm-quota.so"
