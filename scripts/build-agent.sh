#!/usr/bin/env bash
set -euo pipefail

# Build cross-platform standalone cloudpulse-agent binaries
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/bin"
VERSION="${AGENT_VERSION:-1.0.0}"

mkdir -p "${OUT_DIR}"
cd "${ROOT_DIR}/agent"

echo "Building cloudpulse-agent ${VERSION} for linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o "${OUT_DIR}/cloudpulse-agent-linux-amd64" ./cmd/agent

echo "Building cloudpulse-agent ${VERSION} for linux/arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o "${OUT_DIR}/cloudpulse-agent-linux-arm64" ./cmd/agent

echo "Building cloudpulse-agent ${VERSION} for host OS..."
go build -buildvcs=false -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o "${OUT_DIR}/cloudpulse-agent" ./cmd/agent

ls -lh "${OUT_DIR}"/cloudpulse-agent*
