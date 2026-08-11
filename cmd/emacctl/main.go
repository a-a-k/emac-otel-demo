package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/analysis"
	"github.com/a-a-k/emac-otel-demo/internal/calibration"
	"github.com/a-a-k/emac-otel-demo/internal/controller"
	"github.com/a-a-k/emac-otel-demo/internal/evaluation"
	"github.com/a-a-k/emac-otel-demo/internal/evidence"
	"github.com/a-a-k/emac-otel-demo/internal/experiment"
	"github.com/a-a-k/emac-otel-demo/internal/model"
	"github.com/a-a-k/emac-otel-demo/internal/statistics"
	"github.com/a-a-k/emac-otel-demo/internal/validation"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "stage-plan":
		err = stagePlan(os.Args[2:])
	case "decide":
		err = decide(os.Args[2:])
	case "watch-bering":
		err = watchBering(os.Args[2:])
	case "admit":
		err = admit(os.Args[2:])
	case "compile":
		err = compile(os.Args[2:])
	case "reconcile":
		err = reconcile(os.Args[2:])
	case "flag-config":
		err = flagConfig(os.Args[2:])
	case "oracle":
		err = oracle(os.Args[2:])
	case "target-share":
		err = targetShare(os.Args[2:])
	case "reconcile-metrics":
		err = reconcileMetrics(os.Args[2:])
	case "extract-projection":
		err = extractProjection(os.Args[2:])
	case "reconcile-boundary":
		err = reconcileBoundary(os.Args[2:])
	case "calibrate":
		err = calibrate(os.Args[2:])
	case "capacity-check":
		err = capacityCheck(os.Args[2:])
	case "analyze-stage":
		err = analyzeStage(os.Args[2:])
	case "rq1-validate":
		err = rq1Validate(os.Args[2:])
	case "replay":
		err = replay(os.Args[2:])
	case "confirmatory":
		err = confirmatory(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: emacctl <stage-plan|flag-config|calibrate|capacity-check|analyze-stage|rq1-validate|replay|confirmatory|compile|admit|reconcile|reconcile-metrics|reconcile-boundary|extract-projection|oracle|target-share|decide|watch-bering> [flags]")
}

func confirmatory(args []string) error {
	f := flag.NewFlagSet("confirmatory", flag.ContinueOnError)
	var runs pathsFlag
	f.Var(&runs, "run", "offline replay JSON; repeat exactly 40 times")
	out := f.String("out", "-", "confirmatory result JSON path or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	result, err := evaluation.Confirmatory(runs)
	if err != nil {
		return err
	}
	var dst *os.File
	if *out == "-" {
		dst = os.Stdout
	} else {
		dst, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer dst.Close()
	}
	encoder := json.NewEncoder(dst)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func replay(args []string) error {
	f := flag.NewFlagSet("replay", flag.ContinueOnError)
	root := f.String("root", "", "full-sweep analysis root")
	nMax := f.Int("n-max", 0, "candidate or frozen N_max")
	out := f.String("out", "-", "replay JSON path or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *root == "" || *nMax <= 0 {
		return fmt.Errorf("root and n-max are required")
	}
	result, err := evaluation.Replay(*root, *nMax)
	if err != nil {
		return err
	}
	var dst *os.File
	if *out == "-" {
		dst = os.Stdout
	} else {
		dst, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer dst.Close()
	}
	encoder := json.NewEncoder(dst)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func rq1Validate(args []string) error {
	f := flag.NewFlagSet("rq1-validate", flag.ContinueOnError)
	cases := f.Int("cases", 10000, "registered finite-support cases")
	seed := f.Int64("seed", 20270811, "registered generator seed")
	out := f.String("out", "-", "result JSON path or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	result, err := validation.ValidateRQ1(*seed, *cases)
	if err != nil {
		return err
	}
	var dst *os.File
	if *out == "-" {
		dst = os.Stdout
	} else {
		dst, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer dst.Close()
	}
	encoder := json.NewEncoder(dst)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func analyzeStage(args []string) error {
	f := flag.NewFlagSet("analyze-stage", flag.ContinueOnError)
	ledgerPath := f.String("ledger", "", "policy ledger JSONL")
	metricsPath := f.String("metrics", "", "Collector metrics JSON stream")
	planPath := f.String("plan", "", "registered stage plan")
	beringDir := f.String("bering", "", "isolated Bering pipeline directory")
	calibrationPath := f.String("calibration", "", "frozen calibration JSON")
	capacityPath := f.String("capacity", "", "registered capacity result JSON")
	pipeline := f.Float64("pipeline", 1, "trace pipeline proportion")
	current := f.Float64("current-weight", -1, "applied rollout weight")
	target := f.Float64("target-weight", -1, "counterfactual target weight")
	look := f.Int("look", 0, "measured prefix size")
	nMax := f.Int("n-max", 0, "frozen maximum stage evidence")
	reconciled := f.Bool("reconciled", false, "prefix evidence passed registered reconciliation")
	out := f.String("out", "-", "analysis JSON path or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *ledgerPath == "" || *metricsPath == "" || *planPath == "" || *beringDir == "" || *calibrationPath == "" || *capacityPath == "" || *current < 0 || *target < 0 {
		return fmt.Errorf("ledger, metrics, plan, bering, calibration, capacity, current-weight, and target-weight are required")
	}
	result, err := analysis.Analyze(analysis.Input{LedgerPath: *ledgerPath, MetricsPath: *metricsPath, PlanPath: *planPath, BeringDir: *beringDir, CalibrationPath: *calibrationPath, CapacityPath: *capacityPath, Pipeline: *pipeline, CurrentWeight: *current, TargetWeight: *target, Look: *look, NMax: *nMax, Reconciled: *reconciled})
	if err != nil {
		return err
	}
	var dst *os.File
	if *out == "-" {
		dst = os.Stdout
	} else {
		dst, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer dst.Close()
	}
	encoder := json.NewEncoder(dst)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func stagePlan(args []string) error {
	f := flag.NewFlagSet("stage-plan", flag.ContinueOnError)
	seed := f.String("seed", "", "registered run seed")
	run := f.String("run", "", "run id")
	stage := f.String("stage", "", "stage id")
	weight := f.Float64("weight", 0, "rollout weight [0,1]")
	warmup := f.Int("warmup", 200, "warm-up requests")
	measured := f.Int("measured", 1000, "measured requests")
	phase := f.String("phase", "measured", "measured or oracle")
	persona := f.String("persona", "exact-60-40", "exact-60-40 or all-eligible")
	out := f.String("out", "-", "output JSON path or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *seed == "" || *run == "" || *stage == "" {
		return fmt.Errorf("seed, run, and stage are required")
	}
	p := experiment.Phase(*phase)
	if p != experiment.PhaseMeasured && p != experiment.PhaseOracle {
		return fmt.Errorf("phase must be measured or oracle")
	}
	plan, err := experiment.BuildStagePlanWithPersona([]byte(*seed), *run, *stage, *weight, *warmup, *measured, p, *persona)
	if err != nil {
		return err
	}
	var dst *os.File
	if *out == "-" {
		dst = os.Stdout
	} else {
		dst, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer dst.Close()
	}
	enc := json.NewEncoder(dst)
	enc.SetIndent("", "  ")
	return enc.Encode(plan)
}

func decide(args []string) error {
	f := flag.NewFlagSet("decide", flag.ContinueOnError)
	admitted := f.Bool("admitted", false, "evidence was admitted")
	lower := f.String("lower", "", "lower success bound")
	upper := f.String("upper", "", "upper success bound")
	target := f.Float64("target", .95, "journey success target")
	if err := f.Parse(args); err != nil {
		return err
	}
	l, err := strconv.ParseFloat(*lower, 64)
	if err != nil {
		return fmt.Errorf("lower: %w", err)
	}
	u, err := strconv.ParseFloat(*upper, 64)
	if err != nil {
		return fmt.Errorf("upper: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"schema": "emac.decision/v1", "admitted": *admitted, "lower": l, "upper": u, "target": *target, "decision": controller.FullEmaC(*admitted, l, u, *target)})
}

func watchBering(args []string) error {
	f := flag.NewFlagSet("watch-bering", flag.ContinueOnError)
	dir := f.String("dir", "", "Bering state directory")
	stop := f.String("stop-file", "", "path whose creation stops the watcher")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *stop == "" {
		return fmt.Errorf("dir and stop-file are required")
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- evidence.Watch(*dir, *stop, 500*time.Millisecond) }()
	select {
	case err := <-done:
		return err
	case <-signals:
		return nil
	}
}

func admit(args []string) error {
	f := flag.NewFlagSet("admit", flag.ContinueOnError)
	dir := f.String("dir", "", "Bering pipeline directory")
	floor := f.Int("floor", 10, "cumulative trace floor")
	conflict := f.Bool("evidence-conflict", false, "ledger reconciliation found a conflict")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("dir is required")
	}
	windows, err := evidence.LoadArchive(*dir)
	if err != nil {
		return err
	}
	stable, err := evidence.LoadProjection(*dir + string(os.PathSeparator) + "latest-stable-core.json")
	if err != nil {
		return err
	}
	required := []evidence.RequiredEdge{{From: "checkout-policy", To: "cart", Operation: "GetCart"}, {From: "checkout-policy", To: "currency", Operation: "GetSupportedCurrencies"}, {From: "checkout-policy", To: "shipping", Operation: "POST"}, {From: "checkout-policy", To: "frontend", Operation: "POST"}}
	result := evidence.Admit(windows, stable, required, *floor, *conflict)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return err
	}
	if !result.Admitted {
		return fmt.Errorf("evidence is not admitted")
	}
	return nil
}

func compile(args []string) error {
	f := flag.NewFlagSet("compile", flag.ContinueOnError)
	input := f.String("input", "-", "compiler input JSON or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	var src *os.File
	var err error
	if *input == "-" {
		src = os.Stdin
	} else {
		src, err = os.Open(*input)
		if err != nil {
			return err
		}
		defer src.Close()
	}
	var in model.CompileInput
	if err := json.NewDecoder(src).Decode(&in); err != nil {
		return err
	}
	out, err := model.CompileThreeCohort(in)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func reconcile(args []string) error {
	f := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	ledgerPath := f.String("ledger", "", "policy ledger JSONL")
	beringDir := f.String("bering", "", "Bering pipeline directory")
	proportion := f.Float64("proportion", 1, "pipeline proportion")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *ledgerPath == "" || *beringDir == "" {
		return fmt.Errorf("ledger and bering are required")
	}
	result, err := evidence.Reconcile(*ledgerPath, *beringDir, *proportion)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("reconciliation failed")
	}
	return nil
}

func flagConfig(args []string) error {
	f := flag.NewFlagSet("flag-config", flag.ContinueOnError)
	weight := f.Float64("weight", -1, "rollout weight [0,1]")
	out := f.String("out", "-", "output path or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *weight < 0 || *weight > 1 {
		return fmt.Errorf("weight must be in [0,1]")
	}
	eligible := map[string]any{"==": []any{map[string]string{"var": "international"}, true}}
	inBucket := map[string]any{"<": []any{map[string]string{"var": "bucket"}, *weight}}
	targeting := map[string]any{"if": []any{map[string]any{"and": []any{eligible, inBucket}}, "on", "off"}}
	flag := map[string]any{"state": "ENABLED", "defaultVariant": "off", "variants": map[string]bool{"off": false, "on": true}, "targeting": targeting}
	doc := map[string]any{"$schema": "https://flagd.dev/schema/v0/flags.json", "flags": map[string]any{"candidateRouting": flag}}
	var dst *os.File
	var err error
	if *out == "-" {
		dst = os.Stdout
	} else {
		dst, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer dst.Close()
	}
	enc := json.NewEncoder(dst)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func oracle(args []string) error {
	f := flag.NewFlagSet("oracle", flag.ContinueOnError)
	input := f.String("input", "-", "JSON array of cohort counts")
	alpha := f.Float64("alpha", .05, "per-weight alpha")
	target := f.Float64("target", .95, "journey target")
	if err := f.Parse(args); err != nil {
		return err
	}
	var src *os.File
	var err error
	if *input == "-" {
		src = os.Stdin
	} else {
		src, err = os.Open(*input)
		if err != nil {
			return err
		}
		defer src.Close()
	}
	var counts []statistics.CohortCount
	if err := json.NewDecoder(src).Decode(&counts); err != nil {
		return err
	}
	interval, label, err := statistics.StratifiedOracle(counts, *alpha, *target)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"schema": "emac.oracle/v1", "interval": interval, "label": label})
}
func targetShare(args []string) error {
	f := flag.NewFlagSet("target-share", flag.ContinueOnError)
	assigned := f.Int("assigned", 0, "eligible assignments below target bucket")
	eligible := f.Int("eligible", 0, "eligible observations")
	mass := f.Float64("eligibility-mass", .6, "design eligibility mass")
	alpha := f.Float64("alpha", .05, "multiplicity-corrected alpha")
	if err := f.Parse(args); err != nil {
		return err
	}
	result, err := statistics.CounterfactualShare(*assigned, *eligible, *mass, *alpha)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func reconcileMetrics(args []string) error {
	f := flag.NewFlagSet("reconcile-metrics", flag.ContinueOnError)
	ledgerPath := f.String("ledger", "", "policy ledger JSONL")
	metricsPath := f.String("metrics", "", "Collector file-exporter stream")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *ledgerPath == "" || *metricsPath == "" {
		return fmt.Errorf("ledger and metrics are required")
	}
	result, err := evidence.ReconcileResidualMetric(*ledgerPath, *metricsPath)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("metrics reconciliation failed")
	}
	return nil
}

func extractProjection(args []string) error {
	f := flag.NewFlagSet("extract-projection", flag.ContinueOnError)
	input := f.String("input", "", "Bering projection view")
	out := f.String("out", "", "snapshot output path")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *input == "" || *out == "" {
		return fmt.Errorf("input and out are required")
	}
	projection, err := evidence.LoadProjection(*input)
	if err != nil {
		return err
	}
	if !projection.Available || projection.Snapshot == nil {
		return fmt.Errorf("projection %s is unavailable", projection.Name)
	}
	dst, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer dst.Close()
	encoder := json.NewEncoder(dst)
	encoder.SetIndent("", "  ")
	return encoder.Encode(projection.Snapshot)
}

func reconcileBoundary(args []string) error {
	f := flag.NewFlagSet("reconcile-boundary", flag.ContinueOnError)
	ledgerPath := f.String("ledger", "", "policy ledger JSONL")
	k6Log := f.String("k6-log", "", "raw k6 console log")
	deadline := f.Float64("deadline-ms", 0, "frozen journey deadline; zero skips temporal equivalence")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *ledgerPath == "" || *k6Log == "" {
		return fmt.Errorf("ledger and k6-log are required")
	}
	result, err := evidence.ReconcileBoundary(*ledgerPath, *k6Log, *deadline)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("policy/k6 boundary reconciliation failed")
	}
	return nil
}

type pathsFlag []string

func (p *pathsFlag) String() string { return fmt.Sprint([]string(*p)) }
func (p *pathsFlag) Set(value string) error {
	*p = append(*p, value)
	return nil
}

func calibrate(args []string) error {
	f := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	var stable, candidate pathsFlag
	f.Var(&stable, "stable-ledger", "weight-0 ledger; repeat exactly three times")
	f.Var(&candidate, "candidate-ledger", "all-eligible candidate ledger; repeat exactly three times")
	out := f.String("out", "-", "calibration JSON path or -")
	if err := f.Parse(args); err != nil {
		return err
	}
	result, err := calibration.Build(stable, candidate)
	if err != nil {
		return err
	}
	var dst *os.File
	if *out == "-" {
		dst = os.Stdout
	} else {
		dst, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer dst.Close()
	}
	encoder := json.NewEncoder(dst)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func capacityCheck(args []string) error {
	f := flag.NewFlagSet("capacity-check", flag.ContinueOnError)
	summary := f.String("k6-summary", "", "k6 summary-export JSON")
	resources := f.String("resources", "", "docker stats JSONL")
	bering := f.String("bering", "", "archived 100% Bering pipeline")
	rate := f.Float64("rate", 0, "registered ingress rate")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *summary == "" || *resources == "" || *bering == "" || *rate <= 0 {
		return fmt.Errorf("k6-summary, resources, bering, and positive rate are required")
	}
	result, err := evaluation.Capacity(*summary, *resources, *bering, *rate)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("capacity checks failed")
	}
	return nil
}
