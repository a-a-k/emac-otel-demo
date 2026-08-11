#!/usr/bin/env bash
set -euo pipefail

calibration="${EMAC_CALIBRATION:-$EMAC_ROOT/protocol/calibration-v1.json}"
cp "$EMAC_ROOT/deploy/collector/config.yaml" "$EMAC_RESULTS/collector.yaml"
if [[ ! -s "$calibration" ]]; then
  exit 0
fi

grid_csv="$(jq -r '.histogram_grid_ms | map(tostring) | join(",")' "$calibration")"
buckets="$(jq -r '.histogram_grid_ms | map((tostring) + "ms") | "[" + join(", ") + "]"' "$calibration")"
sed -i -E "s#^([[:space:]]*buckets:).*#\1 $buckets#" "$EMAC_RESULTS/collector.yaml"
exported="$EMAC_RESULTS/histogram-grid.env"
printf 'EMAC_HISTOGRAM_GRID_MS=%s\n' "$grid_csv" > "$exported"
