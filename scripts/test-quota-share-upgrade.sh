#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export GOMAXPROCS="${GOMAXPROCS:-2}"
export GOMEMLIMIT="${GOMEMLIMIT:-1200MiB}"
export GOFLAGS="${GOFLAGS:--p=1 -count=1}"
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=1536}"

if command -v pnpm >/dev/null 2>&1; then
  FRONTEND_PKG_MGR=(pnpm --dir "$ROOT_DIR/frontend")
else
  FRONTEND_PKG_MGR=(npm --prefix "$ROOT_DIR/frontend")
fi

echo "[quota-share-upgrade] Go low-resource targeted tests"
(
  cd "$ROOT_DIR/backend"
  go test -run 'TestQuotaShareService(GetKeyUsageUsesDBWindowTotals|CheckLimitsUsesMaxOfRedisAndDBFor5h|CheckLimitsDBFailureFallsBackToRedis|CheckLimitsUsesMaxOfRedisAndDBFor7d|UpdateGlobalWindowKeepsSmallDriftWindow|TryCalibrateCreatesPendingSuggestion|ApplyAndDiscardCalibrationSuggestion)$' ./internal/service
  go test -run '^$' ./internal/handler/admin ./internal/handler ./internal/repository
)

echo "[quota-share-upgrade] Frontend typecheck"
"${FRONTEND_PKG_MGR[@]}" run typecheck

if [ "${RUN_FRONTEND_BUILD:-1}" = "1" ]; then
  echo "[quota-share-upgrade] Frontend build"
  "${FRONTEND_PKG_MGR[@]}" run build
fi

if [ "${RUN_LIVE_SMOKE:-0}" = "1" ]; then
  if [ -z "${BASE_URL:-}" ]; then
    echo "BASE_URL is required when RUN_LIVE_SMOKE=1" >&2
    exit 1
  fi
  echo "[quota-share-upgrade] Live smoke: $BASE_URL/health"
  curl -fsS "$BASE_URL/health" >/dev/null
fi

echo "[quota-share-upgrade] Done"
