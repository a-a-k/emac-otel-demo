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

# k6 runs as a nonroot UID and writes through the bind mount.
touch "$EMAC_RESULTS/k6-summary.json"
chmod 0666 "$EMAC_RESULTS/k6-summary.json"
: > "$EMAC_RESULTS/k6.log"
mkdir -p "$EMAC_RESULTS/looks"

run_k6_segment() {
  local start="$1" end="$2" look="$3"
  local count=$((end - start))
  local duration=$(( (count + EMAC_RATE - 1) / EMAC_RATE ))
  docker run --rm --network opentelemetry-demo \
	  -e EMAC_STAGE_PLAN=/work/stage-plan.json \
	  -e POLICY_URL=http://checkout-policy:8080 \
	  -e FRONTEND_URL=http://frontend:8080 \
    -e EMAC_RATE \
    -e EMAC_REQUEST_START="$start" \
    -e EMAC_REQUEST_END="$end" \
    -e EMAC_DURATION="${duration}s" \
    -v "$EMAC_RESULTS:/work" \
    -v "$EMAC_ROOT/workload:/scripts:ro" \
    grafana/k6:0.57.0 run --log-format raw --log-output stdout --summary-export /work/k6-summary.json /scripts/checkout.js \
    | tee -a "$EMAC_RESULTS/k6.log"
  cp "$EMAC_RESULTS/k6-summary.json" "$EMAC_RESULTS/looks/k6-summary-look-$(printf '%05d' "$look").json"
}

archive_look() {
  local look="$1"
  # tail sampling waits 5s and Bering windows are 30s. The pause closes a
  # complete evidence window without allowing any later request into it.
  sleep 40
  local manifest="$EMAC_RESULTS/looks/look-$(printf '%05d' "$look").json"
  local entries=()
  "$EMAC_RESULTS/emacctl" reconcile-metrics \
    --ledger "$EMAC_RESULTS/policy/ledger.jsonl" \
    --metrics "$EMAC_RESULTS/metrics/metrics.json" \
    > "$EMAC_RESULTS/looks/metrics-reconciliation-look-$(printf '%05d' "$look").json"
  for spec in 100:1 25:.25 05:.05; do
    local pipeline="${spec%%:*}" proportion="${spec##*:}"
    local observation
    observation="$(jq -r '.observation' "$EMAC_RESULTS/bering/$pipeline/latest-raw-window.json")"
    local archive="$EMAC_RESULTS/bering/$pipeline/raw-windows/observation-$(printf '%06d' "$observation").json"
    for _ in $(seq 1 20); do [[ -s "$archive" ]] && break; sleep 1; done
    test -s "$archive"
    "$EMAC_RESULTS/emacctl" reconcile --ledger "$EMAC_RESULTS/policy/ledger.jsonl" --bering "$EMAC_RESULTS/bering/$pipeline" --proportion "$proportion" \
      > "$EMAC_RESULTS/looks/reconciliation-look-$(printf '%05d' "$look")-p${pipeline}.json"
    entries+=("$pipeline" "$observation")
  done
  jq -n --argjson look "$look" \
    --argjson p100 "${entries[1]}" --argjson p25 "${entries[3]}" --argjson p05 "${entries[5]}" \
    '{schema:"emac.evidence-look/v1", look:$look, observation_versions:{"100":$p100,"25":$p25,"05":$p05}}' > "$manifest"
}

if [[ "${EMAC_INCREMENTAL:-false}" == true ]]; then
  warmup="$(jq '[.requests[] | select(.phase == "warmup")] | length' "$EMAC_RESULTS/stage-plan.json")"
  previous=0
  for look in 1000 2000 4000 8000 12000 16000 20000 "$EMAC_N_MAX"; do
    (( look <= EMAC_N_MAX )) || continue
    (( look > previous )) || continue
    start=$((warmup + previous))
    end=$((warmup + look))
    if (( previous == 0 )); then start=0; fi
    run_k6_segment "$start" "$end" "$look"
    archive_look "$look"
    previous="$look"
  done
else
  total="$(jq '.requests | length' "$EMAC_RESULTS/stage-plan.json")"
  measured="$(jq '[.requests[] | select(.phase == "measured" or .phase == "oracle")] | length' "$EMAC_RESULTS/stage-plan.json")"
  run_k6_segment 0 "$total" "$measured"
fi

if [[ "${EMAC_PHASE:-measured}" == oracle ]]; then
  # Oracle-only targets are isolated validation stages. The Collector's
  # measured-trace selectors and policy metric recorder exclude this phase,
  # so no oracle telemetry can enter Bering, Span Metrics, or controller state.
  cleanup_archivers
  trap - EXIT
  boundary_args=(--ledger "$EMAC_RESULTS/policy/ledger.jsonl" --k6-log "$EMAC_RESULTS/k6.log")
  if [[ -n "${EMAC_JOURNEY_DEADLINE_MS:-}" ]]; then
    boundary_args+=(--deadline-ms "$EMAC_JOURNEY_DEADLINE_MS")
  fi
  "$EMAC_RESULTS/emacctl" reconcile-boundary "${boundary_args[@]}" > "$EMAC_RESULTS/boundary-reconciliation.json"
  "$EMAC_RESULTS/emacctl" oracle-ledger \
    --ledger "$EMAC_RESULTS/policy/ledger.jsonl" \
    --phase oracle \
    --weight "$EMAC_WEIGHT" \
    --deadline-ms "$EMAC_JOURNEY_DEADLINE_MS" \
    --n "$EMAC_N_MAX" \
    --out "$EMAC_RESULTS/oracle.json"
  exit 0
fi

./scripts/stage-barrier.sh
sleep 2
cleanup_archivers
trap - EXIT

capacity_summaries=()
for summary in "$EMAC_RESULTS"/looks/k6-summary-look-*.json; do
  [[ -e "$summary" ]] && capacity_summaries+=(--k6-summary "$summary")
done
"$EMAC_RESULTS/emacctl" capacity-check \
  "${capacity_summaries[@]}" \
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
