#!/usr/bin/env bash
set -euo pipefail

# Colors
RED="\033[1;31m"
YELLOW="\033[1;33m"
GREEN="\033[1;32m"
BLUE="\033[1;34m"
CYAN="\033[1;36m"
RESET="\033[0m"

echo -e "${BLUE}=== govulncheck ===${RESET}"
GV_RC=0
set +e
govulncheck ./...
GV_RC=$?
set -e
if [[ $GV_RC -ne 0 ]]; then
  echo -e "${RED}Vulnerabilities found by govulncheck!${RESET}"
else
  echo -e "${GREEN}No vulnerabilities found by govulncheck.${RESET}"
fi

echo -e "${BLUE}=== OSV-Scanner (table) ===${RESET}"
osv-scanner -r . --format table | tee osv.txt >/dev/null || true

# Parse summary
CRIT=$(grep -Eo '\([0-9]+ Critical' osv.txt | grep -Eo '[0-9]+' | head -n1 || true)
HIGH=$(grep -Eo 'Critical, [0-9]+ High' osv.txt | grep -Eo '[0-9]+' | head -n1 || true)
CRIT=${CRIT:-0}
HIGH=${HIGH:-0}

ANY=0
if grep -qE 'Total .* \([0-9]+ (Critical|High|Medium|Low|Unknown)' osv.txt; then
  if ! grep -q '(0 Critical, 0 High, 0 Medium, 0 Low, 0 Unknown)' osv.txt; then
    ANY=1
  fi
fi

# Highlight results
if (( CRIT > 0 )); then
  echo -e "Critical: ${RED}${CRIT}${RESET}"
else
  echo -e "Critical: ${GREEN}0${RESET}"
fi

if (( HIGH > 0 )); then
  echo -e "High: ${YELLOW}${HIGH}${RESET}"
else
  echo -e "High: ${GREEN}0${RESET}"
fi

if (( ANY > 0 && CRIT == 0 && HIGH == 0 )); then
  echo -e "Medium/Low/Unknown vulnerabilities found: ${CYAN}Yes${RESET}"
elif (( ANY == 0 )); then
  echo -e "${GREEN}No vulnerabilities found by OSV-Scanner.${RESET}"
fi

# Export for enforce_policy.sh
cat > vuln_summary.env <<EOF
GV_RC=${GV_RC}
OSV_ANY=${ANY}
OSV_HIGH=${HIGH}
OSV_CRIT=${CRIT}
EOF
