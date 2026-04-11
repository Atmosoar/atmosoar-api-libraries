#!/usr/bin/env bash
set -euo pipefail

# Colors
RED="\033[1;31m"
YELLOW="\033[1;33m"
GREEN="\033[1;32m"
BLUE="\033[1;34m"
CYAN="\033[1;36m"
RESET="\033[0m"

source vuln_summary.env

branch="${GITHUB_REF_NAME:-unknown}"
echo -e "${BLUE}=== Vulnerability Policy Enforcement for branch: ${branch} ===${RESET}"
echo -e "govulncheck rc: ${CYAN}${GV_RC}${RESET}"
echo -e "OSV any: ${CYAN}${OSV_ANY}${RESET}, high: ${CYAN}${OSV_HIGH}${RESET}, crit: ${CYAN}${OSV_CRIT}${RESET}"

hard_fail() {
  echo -e "\n${RED}============================================================${RESET}"
  echo -e "${RED}  ❌  VULNERABILITY POLICY VIOLATED — HARD FAIL               ${RESET}"
  echo -e "${RED}============================================================${RESET}\n"
  exit 1
}

soft_note() {
  echo -e "\n${YELLOW}------------------------------------------------------------${RESET}"
  echo -e "${YELLOW}  ⚠  Vulnerabilities found — soft fail (see scan logs)      ${RESET}"
  echo -e "${YELLOW}------------------------------------------------------------${RESET}\n"
}

if [[ "${branch}" == "main" || "${branch}" == "master" ]]; then
  # release branch: ANY severity from either tool => hard fail
  if [[ "${GV_RC}" != "0" || "${OSV_ANY}" == "1" ]]; then
    hard_fail
  fi
  echo -e "${GREEN}No vulnerabilities found on ${branch}.${RESET}"
else
  # feature branches: HIGH/CRITICAL => hard fail; lower severities => soft fail
  if (( OSV_CRIT > 0 || OSV_HIGH > 0 )); then
    hard_fail
  fi
  if [[ "${GV_RC}" != "0" || "${OSV_ANY}" == "1" ]]; then
    soft_note
  else
    echo -e "${GREEN}No vulnerabilities found on ${branch}.${RESET}"
  fi
fi
