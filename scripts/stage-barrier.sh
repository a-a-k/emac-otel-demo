#!/usr/bin/env bash
set -euo pipefail

# The connector does not export its partial delta interval on Shutdown. Wait
# for a tick that closes after the final span, then require a durable file
# exporter acknowledgement before asking Bering for its forced final flush.
last_span_epoch="$(date +%s)"
sleep 15
test "$(date +%s)" -ge "$((last_span_epoch + 10))"
test -s "$EMAC_RESULTS/metrics/metrics.json"
test "$(stat -c %Y "$EMAC_RESULTS/metrics/metrics.json")" -ge "$last_span_epoch"
"$EMAC_RESULTS/emacctl" reconcile-metrics --ledger "$EMAC_RESULTS/policy/ledger.jsonl" --metrics "$EMAC_RESULTS/metrics/metrics.json" > "$EMAC_RESULTS/metrics/reconciliation.json"

for pipeline in 100 25 05; do
  test -d "$EMAC_RESULTS/bering/$pipeline"
done

docker compose --env-file third_party/opentelemetry-demo/.env -f third_party/opentelemetry-demo/compose.yaml -f deploy/compose.emac.yaml stop -t 30 bering100 bering25 bering05

for pipeline in 100 25 05; do
  test -s "$EMAC_RESULTS/bering/$pipeline/latest-raw-window.json"
  test -s "$EMAC_RESULTS/bering/$pipeline/latest-stable-core.json"
done

# Sheaft is an advisory compatibility consumer, not the latency controller.
# It consumes the Bering-discovered stable snapshot without rediscovery.
mkdir -p "$EMAC_RESULTS/sheaft"
"$EMAC_RESULTS/emacctl" extract-projection \
  --input "$EMAC_RESULTS/bering/100/latest-stable-core.json" \
  --out "$EMAC_RESULTS/sheaft/stable-core.snapshot.json"
go -C third_party/sheaft run ./cmd/sheaft simulate \
  --model "$(realpath "$EMAC_RESULTS/sheaft/stable-core.snapshot.json")" \
  --policy "$(realpath deploy/sheaft/advisory.policy.yaml)" \
  --out "$(realpath -m "$EMAC_RESULTS/sheaft/report.json")" \
  --seed 10962
