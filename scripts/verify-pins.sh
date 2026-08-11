#!/usr/bin/env bash
set -euo pipefail

test "$(git -C third_party/opentelemetry-demo rev-parse HEAD)" = 1755859a9de82c2e5e225be68abc401a5ebf2b4f
test "$(git -C third_party/bering describe --tags --exact-match)" = v1.0.0
test "$(git -C third_party/sheaft describe --tags --exact-match)" = v1.2.0

tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
go run ./cmd/emacctl stage-plan --seed test-seed --run r1 --stage 10 --weight .1 --warmup 0 --measured 2 --out "$tmp/plan.json"
python3 - "$tmp/plan.json" <<'PY'
import json, sys
p = json.load(open(sys.argv[1], encoding="utf-8"))["requests"][1]
assert p["rollout_key"] == "a33b6418-e488-5d97-bfcf-4244d0601156"
assert p["user_id"] == "8d156f3d-6ec4-578d-97fe-f1ded6bc8e36"
assert p["request_id"] == "8051be38-9132-54fc-989a-454f84c3d7e3"
assert abs(p["bucket"] - 0.05963951879416274) < 1e-15
PY
