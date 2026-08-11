#!/usr/bin/env bash
set -euo pipefail

calibration="${EMAC_CALIBRATION:-$EMAC_ROOT/protocol/calibration-v1.json}"
test -s "$calibration"
mkdir -p "$EMAC_RESULTS/analysis"

current_percent="$(jq -r '(.weight * 100) | round' "$EMAC_RESULTS/stage-plan.json")"
case "$current_percent" in
  10) target_percent=25 ;;
  25) target_percent=50 ;;
  50) target_percent=75 ;;
  75) target_percent=100 ;;
  100) target_percent=100 ;;
  *) target_percent="$current_percent" ;;
esac
current="$(python3 -c 'import sys; print(int(sys.argv[1])/100)' "$current_percent")"
target="$(python3 -c 'import sys; print(int(sys.argv[1])/100)' "$target_percent")"

caps_csv="${EMAC_ANALYSIS_CAPS:-$EMAC_N_MAX}"
IFS=',' read -r -a requested_caps <<< "$caps_csv"
for cap in "${requested_caps[@]}"; do
  (( cap >= 1000 && cap <= EMAC_N_MAX )) || continue
  looks=(1000)
  for candidate in 2000 4000 8000; do
    (( candidate <= cap )) && looks+=("$candidate")
  done
  if [[ " ${looks[*]} " != *" $cap "* ]]; then looks+=("$cap"); fi
  for look in "${looks[@]}"; do
    look_manifest="$EMAC_RESULTS/looks/look-$(printf '%05d' "$look").json"
    test -s "$look_manifest"
    for spec in 100:1 25:.25 05:.05; do
      pipeline="${spec%%:*}"; proportion="${spec##*:}"
      observation="$(jq -r --arg pipeline "$pipeline" '.observation_versions[$pipeline]' "$look_manifest")"
      "$EMAC_RESULTS/emacctl" analyze-stage \
        --ledger "$EMAC_RESULTS/policy/ledger.jsonl" \
        --metrics "$EMAC_RESULTS/metrics/metrics.json" \
        --plan "$EMAC_RESULTS/stage-plan.json" \
        --bering "$EMAC_RESULTS/bering/$pipeline" \
        --calibration "$calibration" \
        --capacity "$EMAC_RESULTS/capacity.json" \
        --pipeline "$proportion" \
        --current-weight "$current" \
        --target-weight "$target" \
        --look "$look" \
        --n-max "$cap" \
        --bering-observation "$observation" \
        --reconciled \
        --out "$EMAC_RESULTS/analysis/analysis-cap-$(printf '%05d' "$cap")-look-$(printf '%05d' "$look")-p${pipeline}.json"
    done
  done
done
