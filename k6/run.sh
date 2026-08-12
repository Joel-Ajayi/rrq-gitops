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
#   PROMETHEUS_URL - Prometheus remote write endpoint

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# ── Seed command: create merchants, wallets, and pre-fund ──
if [ "${1:-}" = "seed" ]; then
  echo "══════════════════════════════════════════════════════════════"
  echo "  SEED: Creating 100 merchants & 100,000 wallets in DB"
  echo "══════════════════════════════════════════════════════════════"
  node "${ROOT}/k6/seed-test-data.mjs"
  exit 0
fi

# ── Check for k6 ──
if ! command -v k6 >/dev/null 2>&1; then
  echo "ERROR: k6 is not installed. Please install k6: https://grafana.com/docs/k6/latest/set-up/install-k6/"
  exit 1
fi

SCENARIO="${1:-load}"
TARGET_ENV="${2:-dev}"
VERIFY="true"

# Parse flags
for arg in "$@"; do
  case "$arg" in
    --verify) VERIFY=true ;;
    --no-verify) VERIFY=false ;;
  esac
done

SCRIPT="${ROOT}/k6/scenarios/${SCENARIO}.ts"
if [ ! -f "$SCRIPT" ]; then
  SCRIPT="${ROOT}/k6/scenarios/${SCENARIO}.js"
fi

if [ ! -f "$SCRIPT" ]; then
  echo "ERROR: Scenario script not found: ${SCENARIO}"
  echo "Available scenarios:"
  find "${ROOT}/k6/scenarios" -maxdepth 2 -type f \( -name "*.ts" -o -name "*.js" \) | sed "s|${ROOT}/k6/scenarios/||" | sed 's/^/  /'
  exit 1
fi

mkdir -p "${ROOT}/k6/reports"
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

K6_FLAGS="--env ENV=${TARGET_ENV}"

# Stream to Prometheus Remote Write if URL is provided or running in cluster
if [ -n "${K6_PROMETHEUS_RW_SERVER_URL:-}" ]; then
  K6_FLAGS="$K6_FLAGS --out experimental-prometheus-rw"
  echo "Streaming real-time metrics to Prometheus: ${K6_PROMETHEUS_RW_SERVER_URL}"
fi

k6 run \
  $K6_FLAGS \
  --compatibility-mode=experimental_enhanced \
  --summary-export="${ROOT}/k6/reports/${SCENARIO}-${TARGET_ENV}-summary.json" \
  "$SCRIPT"

END_TS=$(date +%s)
echo ""
echo "══════════════════════════════════════════════════════════════"
echo "  LOAD TEST COMPLETE: ${SCENARIO} (${TARGET_ENV})"
echo "  Duration: $((END_TS - START_TS))s"
echo "══════════════════════════════════════════════════════════════"

# Post-test observability verification
if [ "$VERIFY" = "true" ] && [ -f "${ROOT}/k6/verify.sh" ]; then
  echo ""
  echo "Executing post-test verification..."
  "${ROOT}/k6/verify.sh" "$SCENARIO" "$START_TS" "$END_TS" || true
fi
