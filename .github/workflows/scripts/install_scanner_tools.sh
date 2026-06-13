#!/usr/bin/env bash
set -euo pipefail

# Install/upgrade scanner tools used by the CI pipelines.
go install golang.org/x/vuln/cmd/govulncheck@v1.3.0
go install github.com/google/osv-scanner/cmd/osv-scanner@v1.9.2

# Ensure Go bin is on PATH for this job
echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
