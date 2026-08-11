# EmaC OpenTelemetry Demo: normative scientific protocol

Status: **scientific design frozen**; the `pilot-protocol-v1` implementation
tag remains gated on CI. This document is the sole normative source for the
scientific design. Summaries and generated reports are non-normative.

## 1. Claim and contribution

Working paper title: **Emergence-as-Code: Evidence-Guided Journey SLOs for
Safe Progressive Delivery**.

The phenomenon under study is

\[
\forall \ell\in SLOBoundLeaves(J):SLO_\ell=GREEN
\centernot\implies SLO_J=GREEN.
\]

Component-green/journey-red is a phenomenon and manipulation check, not the
main result. The main result is whether EmaC can make a sound,
evidence-admitted, explainable decision before a rollout, or abstain when the
available evidence is insufficient.

The paper does not presuppose that EmaC outperforms a feature-aware root-CDF
extrapolator. The claimed contribution is a compositional journey model,
conditional sound outer bounds, runtime evidence admission, and causal
selective control without post-stop leakage.

The AST, `Series`/`Cond`, and ordering are declared intent. Bering discovers
and admits runtime edges, bindings, and support; it does not recover the
journey's control semantics.

The soundness theorem is conditional: if simultaneous-valid target-weight
leaf bands and the mixing interval contain their true values, structural
induction over the AST yields an outer bound for the journey CDF.

## 2. System and immutable versions

The canonical implementation repository is public at
`https://github.com/a-a-k/emac-otel-demo`. It contains no prior traces,
scripts, configurations, results, or paper sources.

Pinned dependencies are recorded in `tools.lock.yaml`:

- OpenTelemetry Demo `3.0.0`, commit
  `1755859a9de82c2e5e225be68abc401a5ebf2b4f`;
- Bering `v1.0.0`, schema `1.3.0`;
- Sheaft `v1.2.0`;
- OpenTelemetry Collector Contrib `0.157.0`, Linux/AMD64 manifest
  `sha256:4eb842091c796156d4d3c994eb22ba793590f5723719dbf6b8436cb4dfc17f48`.

Official upstream Demo containers are not modified. The experiment adds a Go
reverse proxy named `checkout-policy`, a Compose overlay, workload code, EmaC,
and evaluation infrastructure. The workload introduces no chaos faults,
service kills, injected sleeps, or weight-dependent ingress changes.

## 3. Workload, identity, and cart setup

Each checkout contains exactly:

- `OLJCESPC7Z`, quantity `1`;
- `66VCHSJNUP`, quantity `1`.

A request is international iff `address.country != "United States"`.
Domestic requests use United States/USD; international requests use
Canada/CAD. Every complete block of ten requests contains exactly six
international and four domestic requests, in a seeded permutation.

Seeds are domain-separated:

```text
identity_seed    = HKDF(run_seed, "identity")
eligibility_seed = HKDF(run_seed, "eligibility")
rollout_seed     = HKDF(run_seed, "rollout")
```

Identifiers are:

```text
rollout_key = UUIDv5(NS_ROLLOUT, rollout_seed || ":" || request_index)
user_id     = UUIDv5(NS_USER, identity_seed || ":" || stage_id || ":" || request_index)
request_id  = UUIDv5(NS_REQUEST, identity_seed || ":" || stage_id || ":" || request_index || ":checkout")
```

Namespaces, encoding, byte order, HKDF, HMAC, and UUID test vectors are frozen
in the protocol YAML. `rollout_key` is stage-independent. `user_id` is
stage-specific and joins Cart with Checkout. `request_id` joins k6, policy,
ledger, and oracle records.

Because `/api/checkout` reads the pre-existing cart, each user first receives
two setup calls to `/api/cart`, one for each fixed product, using the same
`user_id`. Stage execution is:

1. generate all warm-up and measured identities;
2. populate their carts;
3. drain and discard setup telemetry;
4. execute 200 warm-up checkouts;
5. drain/reset evidence systems;
6. begin measured checkout.

Phases are `setup`, `warmup`, `measured`, and `oracle`. Phase is carried as an
entry-span attribute and W3C trace-state value and normalized by the Collector
onto every span. Only `measured` reaches Bering or Span Metrics. Missing or
unknown phase is an integrity failure.

## 4. Feature assignment and runtime branches

The policy computes the single registered bucket

\[
b=\frac{uint64(HMAC\text{-}SHA256(rollout\_seed,rollout\_key)[0:8])}{2^{64}}.
\]

Branch assignment is exactly:

```text
candidate            iff international AND bucket < weight
stable_international iff international AND bucket >= weight
stable_domestic      iff NOT international
```

The policy supplies `international` and this bucket to flagd. Flagd applies
the registered threshold and never computes an independent hash. Flagd,
assignment logging, and estimation of `h` therefore share one implementation
and one set of test vectors.

The candidate branch calls, in order:

```text
policy CLIENT CartService/GetCart
policy CLIENT CurrencyService/GetSupportedCurrencies
policy CLIENT Shipping/POST /get-quote
policy CLIENT Frontend/POST /api/checkout
```

Stable international and stable domestic each call the frontend checkout
once. Candidate results are used: the cart must contain the two fixed items,
the requested currency must be supported, and the shipping quote must be
valid and positive. Failure makes the candidate checkout incorrect; there is
no fallback.

The rollout sequence is `10 -> 25 -> 50 -> 75 -> 100`. At weight 100 the main
60/40 workload has candidate share about 0.6. Capacity characterization uses
an all-eligible Canada/CAD workload, so weight 100 has candidate share 1.

## 5. Three-cohort model

\[
\begin{aligned}
J_c &= Series(R_c,Cart_c,Currency_c,Shipping_c,Frontend_c),\\
J_{si} &= Series(R_{si},Frontend_{si}),\\
J_{sd} &= Series(R_{sd},Frontend_{sd}),\\
J_w &= Cond\!\left(0.6,Cond(h_{target},J_c,J_{si}),J_{sd}\right).
\end{aligned}
\]

Here `h(w)=P(bucket<w | international)` and `q(w)=0.6 h(w)`. The compiler
uses nine CDFs: five candidate leaves, two stable-international leaves, and
two stable-domestic leaves. Residuals participate in composition but are not
service component SLOs. The feature-aware baseline instead uses the three
cohort root CDFs.

## 6. Call ledger and marginal semantics

The per-request ledger records run, stage, phase, request, trace, branch,
operation, intended/attempted/correct/lawful-skipped status, error or invalid
status, span ID, monotonic timestamps, and duration.

- An attempted call without exactly one CLIENT span is an evidence conflict.
- A lawful skip after an earlier branch failure is not a conflict.
- An attempted error or invalid response is an incorrect observation.
- A correct attempted operation has finite duration.

Component success uses attempted operations as denominator. Component p95
uses correct attempted operations. A composition marginal uses every
branch-intended request: attempted errors and lawful skips contribute mass at
mathematical positive infinity. Missing or duplicate attempted spans are
conflicts, not mass.

For branch `b`, the policy computes

\[
R_b=L_{root}-\sum_{\ell\in executed(b)}L_\ell
\]

using the same monotonic timestamps supplied to CLIENT spans. Residuals in
`[-1us,0)` are rounded to zero; values below `-1us` are conflicts. The policy
exports residual histograms plus intended, attempted, and correct counters.

## 7. Component SLO and journey correctness

An operation is GREEN iff its success among attempted calls is at least 99%
and its p95 among correct attempted calls does not exceed its frozen local
deadline.

At weight `w`, branch design masses are

\[
m_c=0.6w,\qquad m_{si}=0.6(1-w),\qquad m_{sd}=0.4.
\]

A stage is component-green iff all SLO-bound operations in positive-design-
mass branches are GREEN. A zero-mass cohort is excluded.

Per root request,

\[
Y_i=1\{correct_i\land L_{root,i}\le D\}.
\]

A correct checkout has HTTP 200, valid response schema, nonempty order and
tracking IDs, and exactly the two requested products with correct quantities.
The policy root is the primary server-side journey boundary. k6 is an
independent external validation oracle and is never an EmaC input.

Pilot reconciliation requires request-ID match at least 99.9% and correctness
agreement at least 99%. Temporal equivalence is claimed only if p95 absolute
duration difference is at most `max(10ms, 0.1D)`.

## 8. Oracle labels

For an applied stage, the oracle sample is the first 1000 `phase=measured`
roots. For a target not applied because of causal BLOCK or terminal REVIEW,
it is the first 1000 `phase=oracle` roots from an isolated stage. Setup and
warm-up never enter an oracle sample.

Exact Clopper-Pearson intervals are built separately for candidate,
stable-international, and stable-domestic success, with per-weight allocation
`0.05/3`. Conditional on the first 1000 cohort counts,

\[
L_{oracle}=\sum_b\pi_bL_b,\qquad U_{oracle}=\sum_b\pi_bU_b.
\]

When `N_b=0`, its interval is `[0,1]` and `pi_b=0`. Labels are

\[
SAFE\iff L_{oracle}\ge0.95,\qquad
UNSAFE\iff U_{oracle}<0.95,
\]

and INDETERMINATE otherwise. The guarantee is per-weight and
design-conditional; aggregate Bernoulli CP is not used.

## 9. Counterfactual target share

\[
\hat h(w)=\frac{N(international\land bucket<w)}{N(international)}.
\]

A multiplicity-corrected exact CP interval `[h_L,h_U]` is used and mapped to
`[q_L,q_U]=0.6[h_L,h_U]`. Full EmaC and the feature-aware baseline use this
same interval. Every decision records current and target weight, h/q values
and intervals, admission, journey bounds, decision, reason, and evidence
digest.

## 10. Transportability

Prediction is conditional on cohort-specific current-weight CDFs equaling
their target-weight counterparts under fixed ingress, independent rollout
hash, no saturation, and no load-induced drift. The experiment does not claim
to prove equivalence.

The drift family has at most `4 transitions * 9 CDFs * 2 tests`; its per-test
allocation is `0.05/(4*9*2)`. Each applicable pair receives a two-sample DKW
test, a simultaneous bootstrap interval for p95 ratio against `[0.90,1.10]`,
and recompilation with target-stage marginals. Results retain the two
orthogonal flags `drift_detected` and `decision_changed`, including all four
combinations. Causal decisions are never changed post hoc.

## 11. Collector and histogram semantics

The pinned Collector uses Delta temporality, a 10-second metrics flush, unit
`ms`, and dimensions for operation, branch, correctness, run, stage, and the
registered evidence-look block.
Only `phase=measured` spans reach evidence pipelines. Unfiltered measured
traces are sampled for Bering; measured `correct=true` spans reach Span
Metrics. Attempted and intended denominators come from the ledger.

After calibration the explicit grid is frozen to 32 equal buckets from 0 to
D and 16 from D to 2D, followed by the normal overflow bucket. Histogram
overflow denotes a correct finite duration above 2D. Error/lawful-skip mass
at mathematical infinity is stored separately. Missing or duplicate
attempted spans remain conflicts.

## 12. Sampling and Bering

Trace-ID sampling is deterministic and nested: `5% subset 25% subset 100%`.
Only the 100% pipeline is active and can call apply. The other two are shadow
controllers that share full-stream marginals but have their own sampled
Bering evidence. The rollout-wide 95% claim applies only to the active
pipeline.

Tail selection delays complete traces by five seconds. Immediately before
the three Bering exporters, the Collector therefore normalizes only those
discovery copies' span end times to processing time. This makes Bering's
30-second windows processing-time evidence windows and prevents selection
delay from being misclassified as late telemetry. The pre-sampling Span
Metrics branch retains original event times and durations; no normalized
timestamp is used for a latency CDF, oracle, or manipulation check.

Each run-stage-pipeline receives an independent Bering process and state.
Every raw window is atomically archived by observation version. At a look,
edge support is the sum of trace counts across non-overlapping archived raw
windows. Admission requires required edges, recurrence in two consecutive
windows, cumulative support at least 10, stable-core membership, allowed
manifest delta, zero dropped and late spans, and no ledger/provenance conflict.
Bering metadata confidence is not a gate.

The first candidate Cart edge is the sentinel. For every sampling pipeline
`p`, root, attempted-edge, and candidate-sentinel counts are reconciled
against ledger roots satisfying the same deterministic sampler predicate.

## 13. Stage barrier

After the last measured span the harness stops load, records the last span end,
waits for a metrics tick whose interval closes after that time, obtains durable
exporter acknowledgement, and verifies an empty exporter queue. For every
operation/branch/run/stage, exported correct histogram count including finite
overflow must equal the correct-attempted ledger count. Residual histograms
are reconciled similarly.

Only then may the Collector shut down. Bering is then gracefully closed to
force its final window flush; the final raw window is archived, observation
version continuity is checked, and final ledger/Bering/assignment
reconciliation is performed. The evidence sink acknowledges a batch only
after atomic durable storage.

## 14. Controller and compiler confidence

Weight 0 is calibration/reference only. Controller state begins with an
unconditional 10% canary. The four possible decisions are `10->25`, `25->50`,
`50->75`, and `75->100`; hence `S_max=4`.

For frozen `N_max`, decision looks are the sorted unique intersection of
`{1000,2000,4000,8000,N_max}` with `[1000,N_max]`. With K looks, nine CDFs,
and h, active Full EmaC allocates

\[
\alpha^*=0.05/(4K(9+1)).
\]

DKW is applied at bucket boundaries with interval-censoring bounds inside a
bucket. The sample unit is a request within a cohort. Formal coverage is
conditional on iid/exchangeable observations within cohort; 30-second block
bootstrap is sensitivity analysis only.

Series uses recursive Makarov outer bounds. Bounds from all contiguous binary
parenthesizations are intersected by taking the maximum lower and minimum
upper bound.

Full EmaC decides:

```text
PASS   iff admitted AND L_J(D) >= 0.95
BLOCK  iff admitted AND U_J(D) < 0.95
REVIEW otherwise
```

Inadmissible evidence always means REVIEW. REVIEW at N_max terminates the
trajectory.

## 15. Calibration and locking

Capacity is tested first at 5 measured checkout/s with the all-eligible
workload, then at 2/s if necessary. A rate is acceptable only with dropped
iterations below 1%, CPU p95 below 70%, memory below 80%, and zero telemetry
drops and late spans. Failure at 2/s makes the protocol infeasible.

Three weight-0 runs characterize D, stable-international, and
stable-domestic. A standalone candidate workload characterizes candidate-only
leaves using the same production path.

Local deadlines equal 1.20 times the maximum calibration p95 among correct
attempted operations. D equals 1.10 times the maximum weight-0 policy-root
p99. Values round upward to 10ms.

After characterization, D, deadlines, rate, histogram grid/unit,
manipulation thresholds, workload, products, seeds, hashes, rollout weights,
and statistical families are frozen. The full sweep cannot retune them.

## 16. Manipulation checks

A run-stage is manipulation-valid only when every SLO-bound operation loses
at most one percentage point of success and has p95 ratio at most 1.10,
residuals satisfy the same correct-rate and p95-ratio limits, achieved ingress
is within 2%, dropped iterations are below 1%, CPU p95 is below 70%, memory is
below 80%, and telemetry drops and late spans are zero. All runs remain in
intention-to-treat analysis and are stratified by manipulation and drift flags.

## 17. Identifiability and N_max

The 10% canary must be SAFE. At least one controlled target among 25, 50, and
75 must be SAFE; all preceding controlled targets form a SAFE prefix; the
next higher target is UNSAFE. These stages must be component-green and
manipulation-valid. Full EmaC must PASS the complete SAFE prefix and BLOCK the
first UNSAFE target. Terminal REVIEW before that transition makes the design
non-identifying.

The feasibility sweep collects to the largest affordable cap and replays
prefixes for `1k,2k,4k,8k,12k,16k,20k`. N_max is the smallest value satisfying
the entire criterion and stage runtime limit. If none qualifies, the design is
recorded as non-identifying and is not retuned.

Stage runtime includes two cart setup calls for every warm-up and measured
user, measured checkout time, startup, and drain, and must remain below 5.5h
on a GitHub-hosted job.

## 18. Baseline state machines

All methods have states COLLECT, PASS_ADVANCE, STOP_BLOCK, STOP_REVIEW, and
DONE. Intermediate REVIEW continues within the stage; REVIEW at N_max stops.

- Local uses reconciled ledger component counts and correct-only Span Metrics.
  It has no confidence allocation: PASS iff all positive-mass required
  operations are GREEN, BLOCK if any is RED, and REVIEW on UNKNOWN. UNKNOWN
  means missing data, failed reconciliation, or no attempts for a positive-
  mass required operation.
- Reactive uses current-weight cohort-stratified root outcomes with allocation
  `0.05/(4*K*3)`. It applies the 0.95 lower/upper rule to the current mixture
  and reviews integrity failures or non-constructible intervals.
- Feature-aware uses current Root_c, Root_si, Root_sd and target h, with
  allocation `0.05/(4*K*4)`. It applies the target-mixture 0.95 rule and
  reviews integrity failures or missing positive-mass cohort evidence.
- Full EmaC uses admitted Bering bindings, nine marginals, and target h.
- Eager EmaC uses raw-window bindings without admission but the same math.
- Oracle-model EmaC uses ground-truth AST/bindings and target share but the
  same marginal uncertainty.
- Sampling shadows repeat Full EmaC using pipeline-specific admission and
  never apply a weight.

Every method has separate state and evidence cursor and cannot read later
stages after its own terminal decision.

## 19. Causal and confirmatory evaluation

The causal pilot begins with unconditional 10%. Only active Full EmaC controls
the rollout. After BLOCK or terminal REVIEW, the target is evaluated in a
fresh isolated oracle-only stage with 200 warm-up and 1000 oracle roots. These
records never enter Bering, Span Metrics, or controller state.

Confirmatory comparison uses a fixed full sweep at reference 0 and rollout
weights 10,25,50,75,100. Every run-weight is a fresh Actions job with shared
run seed, rollout keys, and eligibility schedule, and produces an immutable
artifact. Offline replay begins at 10, follows each method's own virtual
trajectory, and prohibits evidence after its virtual stop.

There are 40 paired run seeds.

## 20. Outcomes and statistics

The co-primary outcomes are

\[
A=1\{\text{no oracle-UNSAFE target received PASS}\},
\]

\[
Z=1\{\text{the entire oracle-SAFE prefix received PASS and the first UNSAFE
target received BLOCK or REVIEW}\}.
\]

No-UNSAFE gives `A=1,Z=0`. INDETERMINATE inside the required prefix gives
`Z=0`. Runs are never excluded; BLOCK and REVIEW are reported separately.

The confirmatory Holm family contains exactly six two-sided exact McNemar
tests without continuity correction: Full EmaC versus Local, Reactive, and
Feature-aware on A and Z. Holm controls FWER at 0.05. With 40 pairs,
discordance probability 0.5, directional split 0.9/0.1, and first threshold
0.05/6, registered power is approximately 0.8822.

Secondary analyses include paired cluster bootstrap with 10,000 resamples,
30-second block-bootstrap sensitivity, automation coverage, abstention,
unsafe PASS, BLOCK/REVIEW decomposition, false BLOCK, evidence-to-decision,
bound width/coverage, h/q error, and drift flags.

## 21. Research questions

- RQ1: implementation correctness and tightness of the outer bounds.
- RQ2: pre-deployment selective safety of Full EmaC.
- RQ3: admission robustness under 100/25/5% sampling.
- RQ4: feasibility and scalability; moved to appendix if space requires.

RQ1 uses 10,000 seeded finite-support cases, AST size 2-5, at most eight
support points, and Series, Cond, Parallel, Race, and Timeout. Exact joint CDFs
must lie within EmaC bounds. LP-optimal tightness comparison is limited to
2-4 leaves.

Sheaft is a secondary advisory configuration, not discovery and not the cause
of the primary latency BLOCK. It may be reported in the appendix/artifact.

## 22. Freeze sequence

1. Commit this document and the machine-readable pre-calibration protocol.
2. Implement proxy, workload, ledger, telemetry, compiler, state machines,
   and workflows; pass CI/property/smoke tests.
3. Create annotated tag `pilot-protocol-v1`.
4. Run capacity, weight-0 calibration, and candidate characterization.
5. Freeze numeric D, deadlines, ingress, grid, and manipulation thresholds.
6. Run the feasibility sweep without retuning and select the minimum valid
   N_max.
7. If no value qualifies, record non-identifying and stop.
8. Write numeric parameters, provenance, run IDs, and digests to
   `protocol-v1.yaml` and create annotated tag `protocol-v1`.
9. Run 40 paired confirmatory full sweeps and publish an immutable
   reproduction bundle.

The target is a SEAMS 2027 full research paper (10 content pages plus two
reference pages). RQ4 and Sheaft move to the appendix/artifact first if space
is constrained.
