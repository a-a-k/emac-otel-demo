#!/usr/bin/env bash
set -euo pipefail

weight_fraction="$(python3 -c 'import sys; print(int(sys.argv[1])/100)' "$EMAC_WEIGHT")"
export EMAC_ROOT="${EMAC_ROOT:-$GITHUB_WORKSPACE}"
export EMAC_RESULTS="${EMAC_RESULTS:-$GITHUB_WORKSPACE/results}"
: "${EMAC_PHASE:=measured}"
export EMAC_PHASE
: "${EMAC_RUN_SEED:=feasibility-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}}"
export EMAC_RUN_SEED
mkdir -p "$EMAC_RESULTS"/{policy,metrics,flagd,sheaft,bering/{100,25,05}}
# checkout-policy runs as the distroless nonroot UID. GitHub's bind-mounted
# results directory is otherwise owned by the runner and not writable.
chmod 0777 "$EMAC_RESULTS/policy"
./scripts/render-collector-config.sh
if [[ -s "$EMAC_RESULTS/histogram-grid.env" ]]; then
  source "$EMAC_RESULTS/histogram-grid.env"
  export EMAC_HISTOGRAM_GRID_MS
fi

go run ./cmd/emacctl stage-plan --seed "$EMAC_RUN_SEED" --run "${EMAC_RUN_ID:-feasibility-${GITHUB_RUN_ID}}" --stage "$EMAC_WEIGHT" --weight "$weight_fraction" --warmup 200 --measured "$EMAC_N_MAX" --phase "$EMAC_PHASE" --persona "${EMAC_PERSONA_MODE:-exact-60-40}" --out "$EMAC_RESULTS/stage-plan.json"
go run ./cmd/emacctl flag-config --weight "$weight_fraction" --out "$EMAC_RESULTS/flagd/demo.flagd.json"
export EMAC_WEIGHT="$weight_fraction"
export DEMO_VERSION=3.0.0
docker compose --env-file third_party/opentelemetry-demo/.env -f third_party/opentelemetry-demo/compose.yaml -f deploy/compose.emac.yaml build checkout-policy
docker compose --env-file third_party/opentelemetry-demo/.env -f third_party/opentelemetry-demo/compose.yaml -f deploy/compose.emac.yaml up -d --no-build --wait \
  --scale load-generator=0 --scale flagd-ui=0 --scale telemetry-docs=0 --scale frontend-proxy=0
trap 'docker compose --env-file third_party/opentelemetry-demo/.env -f third_party/opentelemetry-demo/compose.yaml -f deploy/compose.emac.yaml down --volumes' EXIT

total=$((200 + EMAC_N_MAX)); duration=$(( (total + EMAC_RATE - 1) / EMAC_RATE )); export EMAC_DURATION="${duration}s"
if [[ -s "${EMAC_CALIBRATION:-$EMAC_ROOT/protocol/calibration-v1.json}" ]]; then
  export EMAC_JOURNEY_DEADLINE_MS="$(jq -r '.journey_deadline_ms' "${EMAC_CALIBRATION:-$EMAC_ROOT/protocol/calibration-v1.json}")"
fi
if [[ "$EMAC_PHASE" == measured && -s "${EMAC_CALIBRATION:-$EMAC_ROOT/protocol/calibration-v1.json}" ]]; then
  export EMAC_INCREMENTAL=true
fi
./scripts/run-workload.sh
if [[ "$EMAC_PHASE" == measured && "${EMAC_ANALYZE:-true}" == true && -s "${EMAC_CALIBRATION:-$EMAC_ROOT/protocol/calibration-v1.json}" ]]; then
  ./scripts/analyze-stage.sh
fi
