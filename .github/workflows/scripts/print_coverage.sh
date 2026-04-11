#!/usr/bin/env bash
set -euo pipefail

# Colors
BLUE="\033[1;34m"
GREEN="\033[1;32m"
RESET="\033[0m"

# Overall coverage
echo -e "${BLUE}=== Overall project coverage ===${RESET}"
total_cov=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo -e "Total coverage: ${GREEN}${total_cov}${RESET}"
