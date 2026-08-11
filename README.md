# EmaC OpenTelemetry Demo

Reproducible evaluation of evidence-guided, compositional journey-SLO control
on the OpenTelemetry Demo.

The normative scientific design is
[`protocol/SCIENTIFIC_PROTOCOL.md`](protocol/SCIENTIFIC_PROTOCOL.md). The
canonical public repository is
[`a-a-k/emac-otel-demo`](https://github.com/a-a-k/emac-otel-demo).

The repository contains the independent experiment, not the manuscript's old
replay artifacts. OpenTelemetry Demo, Bering, and Sheaft are pinned submodules;
all evaluation runs are defined as GitHub Actions workflows.

## Implemented experiment path

- `checkout-policy` is an HTTP journey boundary in front of the unchanged
  frontend. It evaluates the registered flagd rule, executes candidate
  preclearance, emits CLIENT spans, and writes a per-call ledger.
- `emacctl` materializes deterministic stage identities, flagd configuration,
  Bering archives/admission, ledger reconciliation, DKW/Makarov compilation,
  oracle labels, and controller decisions.
- Sheaft consumes the admitted Bering stable-core snapshot as a pinned
  compatibility/advisory analysis; it is not allowed to redefine latency
  composition or issue the active rollout decision.
- the Collector feeds full-stream span metrics before nested `5⊂25⊂100`
  trace sampling; only the 100% pipeline is active.
- `integration` runs a small end-to-end stage. `feasibility-pilot` refuses to
  start until the commit is tagged `pilot-protocol-v1`.

## Local checks

```text
git submodule update --init --recursive
go test ./...
go vet ./...
```

Docker execution is intentionally identical to CI:

```text
docker compose --env-file third_party/opentelemetry-demo/.env \
  -f third_party/opentelemetry-demo/compose.yaml \
  -f deploy/compose.emac.yaml config
```

## Freeze status

The scientific design is frozen in
[`protocol/SCIENTIFIC_PROTOCOL.md`](protocol/SCIENTIFIC_PROTOCOL.md), with
machine-readable constants in [`protocol-v1.yaml`](protocol/protocol-v1.yaml).
Do not run the official feasibility pilot before `pilot-protocol-v1` exists;
do not fill calibrated fields or create `protocol-v1` until feasibility passes.
