#!/usr/bin/env bash
set -euo pipefail

stop_file="$EMAC_RESULTS/stop-archivers"
rm -f -- "$stop_file"
go build -o "$EMAC_RESULTS/emacctl" ./cmd/emacctl
pids=()
for pipeline in 100 25 05; do
  "$EMAC_RESULTS/emacctl" watch-bering --dir "$EMAC_RESULTS/bering/$pipeline" --stop-file "$stop_file" &
  pids+=("$!")
done
./scripts/monitor-resources.sh &
pids+=("$!")

cleanup_archivers() {
  touch "$stop_file"
  for pid in "${pids[@]}"; do wait "$pid" || true; done
}
trap cleanup_archivers EXIT

docker run --rm --network opentelemetry-demo \
	-e EMAC_STAGE_PLAN=/work/stage-plan.json \
	-e POLICY_URL=http://checkout-policy:8080 \
	-e FRONTEND_URL=http://frontend:8080 \
  -e EMAC_RATE \
  -e EMAC_DURATION \
  -v "$EMAC_RESULTS:/work" \
  -v "$EMAC_ROOT/workload:/scripts:ro" \
  grafana/k6:0.57.0 run --log-format raw --log-output stdout --summary-export /work/k6-summary.json /scripts/checkout.js \
  | tee "$EMAC_RESULTS/k6.log"

./scripts/stage-barrier.sh
sleep 2
cleanup_archivers
trap - EXIT

"$EMAC_RESULTS/emacctl" capacity-check \
  --k6-summary "$EMAC_RESULTS/k6-summary.json" \
  --resources "$EMAC_RESULTS/resources.jsonl" \
  --bering "$EMAC_RESULTS/bering/100" \
  --rate "$EMAC_RATE" > "$EMAC_RESULTS/capacity.json"

boundary_args=(--ledger "$EMAC_RESULTS/policy/ledger.jsonl" --k6-log "$EMAC_RESULTS/k6.log")
if [[ -n "${EMAC_JOURNEY_DEADLINE_MS:-}" ]]; then
  boundary_args+=(--deadline-ms "$EMAC_JOURNEY_DEADLINE_MS")
fi
"$EMAC_RESULTS/emacctl" reconcile-boundary "${boundary_args[@]}" > "$EMAC_RESULTS/boundary-reconciliation.json"

for spec in 100:1 25:.25 05:.05; do
  pipeline="${spec%%:*}"; proportion="${spec##*:}"
  "$EMAC_RESULTS/emacctl" reconcile --ledger "$EMAC_RESULTS/policy/ledger.jsonl" --bering "$EMAC_RESULTS/bering/$pipeline" --proportion "$proportion" > "$EMAC_RESULTS/bering/$pipeline/reconciliation.json"
done
