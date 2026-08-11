#!/usr/bin/env bash
set -euo pipefail

output="$EMAC_RESULTS/resources.jsonl"
stop_file="$EMAC_RESULTS/stop-archivers"
: > "$output"
while [[ ! -e "$stop_file" ]]; do
  epoch="$(date +%s)"
  docker stats --no-stream --format '{{json .}}' \
    | jq -c --argjson epoch "$epoch" '. + {epoch: $epoch}' >> "$output"
  sleep 10
done
