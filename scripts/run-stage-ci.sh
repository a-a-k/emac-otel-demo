#!/usr/bin/env bash
set -euo pipefail

weight_fraction="$(python3 -c 'import sys; print(int(sys.argv[1])/100)' "$EMAC_WEIGHT")"
export EMAC_ROOT="$GITHUB_WORKSPACE"
export EMAC_RESULTS="$GITHUB_WORKSPACE/results"
export EMAC_RUN_SEED="feasibility-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"
mkdir -p "$EMAC_RESULTS"/{policy,metrics,flagd,sheaft,bering/{100,25,05}}

go run ./cmd/emacctl stage-plan --seed "$EMAC_RUN_SEED" --run "feasibility-${GITHUB_RUN_ID}" --stage "$EMAC_WEIGHT" --weight "$weight_fraction" --warmup 200 --measured "$EMAC_N_MAX" --persona "${EMAC_PERSONA_MODE:-exact-60-40}" --out "$EMAC_RESULTS/stage-plan.json"
go run ./cmd/emacctl flag-config --weight "$weight_fraction" --out "$EMAC_RESULTS/flagd/demo.flagd.json"
export EMAC_WEIGHT="$weight_fraction"
docker compose --env-file third_party/opentelemetry-demo/.env -f third_party/opentelemetry-demo/compose.yaml -f deploy/compose.emac.yaml up -d --build --wait
trap 'docker compose --env-file third_party/opentelemetry-demo/.env -f third_party/opentelemetry-demo/compose.yaml -f deploy/compose.emac.yaml down --volumes' EXIT

total=$((200 + EMAC_N_MAX)); duration=$(( (total + EMAC_RATE - 1) / EMAC_RATE )); export EMAC_DURATION="${duration}s"
./scripts/run-workload.sh
