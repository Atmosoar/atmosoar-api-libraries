#!/usr/bin/env bash
set -euo pipefail

# Install/upgrade scanner tools used by the CI pipelines.
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/google/osv-scanner/cmd/osv-scanner@latest

# Ensure Go bin is on PATH for this job
echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
