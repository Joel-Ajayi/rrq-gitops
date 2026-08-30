#!/usr/bin/env bash
# k6/run.sh — Enterprise TypeScript k6 scenario runner with telemetry and observability verification
#
# Usage: ./k6/run.sh <scenario> [env] [--verify] [--no-verify]
# Examples:
#   ./k6/run.sh smoke dev
#   ./k6/run.sh breakpoint dev
#   ./k6/run.sh load prod
#   ./k6/run.sh stress prod
#   ./k6/run.sh seed
#
# Environment Variables:
#   ENV            - Environment profile: dev or prod (default: dev)
#   BASE_URL       - Target API Gateway URL override

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# ── Seed command: create merchants and wallets ──
if [ "${1:-}" = "seed" ]; then
  echo "══════════════════════════════════════════════════════════════"
  echo "  SEED: Creating test merchants & wallets in DB"
  echo "══════════════════════════════════════════════════════════════"
  NODE_TLS_REJECT_UNAUTHORIZED=0 node "${ROOT}/load-tests/seed-test-data.mts"
  exit 0
fi

# ── Deposit / Fund command: pre-fund all test wallets ──
if [ "${1:-}" = "deposit" ] || [ "${1:-}" = "fund" ]; then
  echo "══════════════════════════════════════════════════════════════"
  echo "  DEPOSIT: Pre-funding all test wallets in DB"
  echo "══════════════════════════════════════════════════════════════"
  NODE_TLS_REJECT_UNAUTHORIZED=0 node "${ROOT}/load-tests/deposit-test-data.mts"
  exit 0
fi

# ── Refresh tokens command: acquire fresh JWTs for all merchants ──
if [ "${1:-}" = "refresh" ] || [ "${1:-}" = "refresh-tokens" ]; then
  echo "══════════════════════════════════════════════════════════════"
  echo "  REFRESH: Refreshing JWT tokens for all merchants"
  echo "══════════════════════════════════════════════════════════════"
  NODE_TLS_REJECT_UNAUTHORIZED=0 node "${ROOT}/load-tests/refresh-tokens.mts"
  exit 0
fi

# ── Check for k6 ──
if ! command -v k6 >/dev/null 2>&1; then
  echo "ERROR: k6 is not installed. Please install k6: https://grafana.com/docs/k6/latest/set-up/install-k6/"
  exit 1
fi

SCENARIO="${1:-smoke}"
TARGET_ENV="${2:-dev}"
VERIFY="true"

# Parse flags
for arg in "$@"; do
  case "$arg" in
    --verify) VERIFY=true ;;
    --no-verify) VERIFY=false ;;
  esac
done

SCRIPT="${ROOT}/load-tests/scenarios/${SCENARIO}.ts"
if [ ! -f "$SCRIPT" ]; then
  SCRIPT="${ROOT}/load-tests/scenarios/${SCENARIO}.js"
fi

if [ ! -f "$SCRIPT" ]; then
  echo "ERROR: Scenario script not found: ${SCENARIO}"
  echo "Available scenarios:"
  find "${ROOT}/load-tests/scenarios" -maxdepth 2 -type f \( -name "*.ts" -o -name "*.js" \) | sed "s|${ROOT}/load-tests/scenarios/||" | sed 's/^/  /'
  exit 1
fi

mkdir -p "${ROOT}/load-tests/reports"
START_TS=$(date +%s)

echo ""
echo "══════════════════════════════════════════════════════════════"
echo "  RRQ k6 TEST EXECUTION"
echo "  Scenario:    ${SCENARIO}"
echo "  Environment: ${TARGET_ENV}"
echo "  Script:      ${SCRIPT}"
echo "  Start Time:  $(date -d @"$START_TS" '+%Y-%m-%d %H:%M:%S')"
echo "══════════════════════════════════════════════════════════════"
echo ""

K6_FLAGS="--env ENV=${TARGET_ENV} --insecure-skip-tls-verify"

k6 run \
  $K6_FLAGS \
  --compatibility-mode=experimental_enhanced \
  --summary-export="${ROOT}/load-tests/reports/${SCENARIO}-${TARGET_ENV}-summary.json" \
  "$SCRIPT"

END_TS=$(date +%s)
echo ""
echo "══════════════════════════════════════════════════════════════"
echo "  LOAD TEST COMPLETE: ${SCENARIO} (${TARGET_ENV})"
echo "  Duration: $((END_TS - START_TS))s"
echo "══════════════════════════════════════════════════════════════"

# Post-test observability verification
if [ "$VERIFY" = "true" ] && [ -f "${ROOT}/load-tests/verify.sh" ]; then
  echo ""
  echo "Executing post-test verification..."
  "${ROOT}/load-tests/verify.sh" "$SCENARIO" "$START_TS" "$END_TS" || true
fi
