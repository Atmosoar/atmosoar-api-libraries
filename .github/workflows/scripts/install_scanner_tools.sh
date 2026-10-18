#!/usr/bin/env bash
set -euo pipefail

# Install/upgrade scanner tools used by the CI pipelines.
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
go install github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.7.0

# Ensure Go bin is on PATH for this job
echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
