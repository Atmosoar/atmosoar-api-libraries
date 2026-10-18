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

echo -e "${BLUE}=== OSV-Scanner ===${RESET}"
# osv-scanner v2: exit 0 = clean, 1 = vulnerabilities found, anything else is
# a scanner failure. Counts come from the JSON report (never from the table
# layout, which changes between releases), and an unreadable report fails
# closed via OSV_ERR so a broken scanner cannot turn the gate green.
OSV_RC=0
set +e
osv-scanner scan source -r . --format json > osv.json
OSV_RC=$?
set -e

OSV_ERR=0
ANY=0
CRIT=0
HIGH=0
if [[ $OSV_RC -ne 0 && $OSV_RC -ne 1 ]] || ! jq -e '.results | type == "array"' osv.json >/dev/null 2>&1; then
  echo -e "${RED}OSV-Scanner failed (rc ${OSV_RC}) or wrote no readable report.${RESET}"
  OSV_ERR=1
else
  # One group = one vulnerability (aliases merged); max_severity is a CVSS
  # base score, empty when the advisory carries none.
  read -r ANY CRIT HIGH < <(jq -r '
    [.results[].packages[]?.groups[]? | (.max_severity // "" | tonumber? // 0)] as $s
    | "\($s | length) \([$s[] | select(. >= 9)] | length) \([$s[] | select(. >= 7 and . < 9)] | length)"
  ' osv.json)
  jq -r '.results[].packages[]? | .package as $p | .groups[]?
    | "  \($p.name)@\($p.version): \(.ids | join(", ")) (severity \(.max_severity // "" | if . == "" then "unknown" else . end))"' osv.json
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
OSV_ANY=$(( ANY > 0 ? 1 : 0 ))
OSV_ERR=${OSV_ERR}
OSV_HIGH=${HIGH}
OSV_CRIT=${CRIT}
EOF
