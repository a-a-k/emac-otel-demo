#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}"
: "${EMAC_N_MAX:?EMAC_N_MAX is required}"
: "${EMAC_RATE:?EMAC_RATE is required}"
: "${EMAC_RUN_SEED:?EMAC_RUN_SEED is required}"
: "${EMAC_RUN_ID:?EMAC_RUN_ID is required}"

root="${EMAC_CAUSAL_RESULTS:-$GITHUB_WORKSPACE/causal-results}"
calibration="${EMAC_CALIBRATION:-$GITHUB_WORKSPACE/protocol/calibration-v1.json}"
test -s "$calibration"
mkdir -p "$root"
: > "$root/decisions.jsonl"
: > "$root/oracles.jsonl"

segments=0
for look in 1000 2000 4000 8000 12000 16000 20000; do
  (( look <= EMAC_N_MAX )) && segments=$((segments + 1))
done
# Four decision stages plus one fixed 1000-root evaluation stage must fit a
# single hosted job. The bound includes registered pauses and a conservative
# three-minute startup/drain allowance per isolated stack.
stage_seconds=$(( (EMAC_N_MAX + 200 + EMAC_RATE - 1) / EMAC_RATE + segments * 40 + 180 ))
final_seconds=$(( (1200 + EMAC_RATE - 1) / EMAC_RATE + 180 ))
trajectory_seconds=$((4 * stage_seconds + final_seconds))
if (( trajectory_seconds >= 21000 )); then
  echo "registered causal trajectory bound ${trajectory_seconds}s does not fit the six-hour hosted-job limit" >&2
  exit 1
fi

weights=(10 25 50 75 100)
terminal=false

run_stage() {
  local weight="$1" phase="$2" measured="$3" analyze="$4" destination="$5"
  EMAC_ROOT="$GITHUB_WORKSPACE" \
  EMAC_RESULTS="$destination" \
  EMAC_CALIBRATION="$calibration" \
  EMAC_WEIGHT="$weight" \
  EMAC_PHASE="$phase" \
  EMAC_N_MAX="$measured" \
  EMAC_RATE="$EMAC_RATE" \
  EMAC_RUN_SEED="$EMAC_RUN_SEED" \
  EMAC_RUN_ID="$EMAC_RUN_ID" \
  EMAC_ANALYZE="$analyze" \
  "$GITHUB_WORKSPACE/scripts/run-stage-ci.sh"
}

for transition in 0 1 2 3; do
  current="${weights[$transition]}"
  target="${weights[$((transition + 1))]}"
  stage_dir="$root/weight-$(printf '%03d' "$current")"
  run_stage "$current" measured "$EMAC_N_MAX" true "$stage_dir"

  selected=""
  while IFS= read -r analysis; do
    decision="$(jq -r '.decisions.FullEmaC' "$analysis")"
    selected="$analysis"
    if [[ "$decision" != REVIEW ]]; then
      break
    fi
  done < <(find "$stage_dir/analysis" -maxdepth 1 -type f -name "analysis-cap-$(printf '%05d' "$EMAC_N_MAX")-look-*-p100.json" | sort)
  test -n "$selected"
  decision="$(jq -r '.decisions.FullEmaC' "$selected")"
  jq -c --argjson current "$current" --argjson target "$target" \
    '{schema:"emac.causal-decision/v1", current_weight:$current, target_weight:$target, look, decision:.decisions.FullEmaC, admitted:.admission.admitted, lower:.full_emac_bound.lower_at_deadline, upper:.full_emac_bound.upper_at_deadline, evidence_scope, evidence_cutoff}' \
    "$selected" >> "$root/decisions.jsonl"
  jq -c --argjson weight "$current" \
    '{weight:$weight, source:"measured", label:.evaluation_oracle.label, interval:.evaluation_oracle.interval}' \
    "$selected" >> "$root/oracles.jsonl"

  if [[ "$decision" == PASS ]]; then
    continue
  fi

  oracle_dir="$root/oracle-weight-$(printf '%03d' "$target")"
  run_stage "$target" oracle 1000 false "$oracle_dir"
  jq -c --argjson weight "$target" \
    '{weight:$weight, source:"isolated-oracle", label, interval}' \
    "$oracle_dir/oracle.json" >> "$root/oracles.jsonl"
  terminal=true
  break
done

if [[ "$terminal" == false ]]; then
  # The final PASS applies 100%. There is no subsequent controller decision,
  # but the applied target still receives its fixed measured oracle sample.
  final_dir="$root/weight-100"
  run_stage 100 measured 1000 false "$final_dir"
  deadline="$(jq -r '.journey_deadline_ms' "$calibration")"
  go run ./cmd/emacctl oracle-ledger \
    --ledger "$final_dir/policy/ledger.jsonl" \
    --phase measured \
    --weight 1 \
    --deadline-ms "$deadline" \
    --n 1000 \
    --out "$final_dir/oracle.json"
  jq -c '{weight:100, source:"measured", label, interval}' "$final_dir/oracle.json" >> "$root/oracles.jsonl"
fi

jq -s '.' "$root/decisions.jsonl" > "$root/decisions.json"
jq -s '.' "$root/oracles.jsonl" > "$root/oracles.json"
jq -n \
  --arg run_id "$EMAC_RUN_ID" \
  --argjson n_max "$EMAC_N_MAX" \
  --argjson rate "$EMAC_RATE" \
  --argjson runtime_bound_seconds "$trajectory_seconds" \
  --argjson terminal "$terminal" \
  --slurpfile decisions "$root/decisions.json" \
  --slurpfile oracles "$root/oracles.json" \
  '{schema:"emac.causal-pilot/v1", run_id:$run_id, n_max:$n_max, ingress:$rate, runtime_bound_seconds:$runtime_bound_seconds, terminal_before_100:$terminal, decisions:$decisions[0], oracles:$oracles[0]}' \
  > "$root/causal-pilot.json"
