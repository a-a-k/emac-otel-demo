package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/calibration"
	"github.com/a-a-k/emac-otel-demo/internal/controller"
	"github.com/a-a-k/emac-otel-demo/internal/evidence"
	"github.com/a-a-k/emac-otel-demo/internal/experiment"
	"github.com/a-a-k/emac-otel-demo/internal/ledger"
	"github.com/a-a-k/emac-otel-demo/internal/model"
	"github.com/a-a-k/emac-otel-demo/internal/statistics"
)

var branchOperations = map[string][]string{
	"candidate":            {"policy.residual", "CartService/GetCart", "CurrencyService/GetSupportedCurrencies", "Shipping/POST get-quote", "Frontend/POST api/checkout"},
	"stable_international": {"policy.residual", "Frontend/POST api/checkout"},
	"stable_domestic":      {"policy.residual", "Frontend/POST api/checkout"},
}

type Input struct {
	LedgerPath      string
	MetricsPath     string
	PlanPath        string
	BeringDir       string
	CalibrationPath string
	Pipeline        float64
	CurrentWeight   float64
	TargetWeight    float64
	Look            int
	NMax            int
	Reconciled      bool
}

type LeafEvidence struct {
	Intended         int        `json:"intended"`
	CorrectAttempted int        `json:"correct_attempted"`
	Band             model.Band `json:"band"`
}

type Result struct {
	Schema           string                       `json:"schema"`
	RunID            string                       `json:"run_id"`
	StageID          string                       `json:"stage_id"`
	Pipeline         float64                      `json:"pipeline"`
	CurrentWeight    float64                      `json:"current_weight"`
	TargetWeight     float64                      `json:"target_weight"`
	Look             int                          `json:"look"`
	AlphaStar        float64                      `json:"alpha_star"`
	TargetShare      statistics.TargetShare       `json:"target_share"`
	Admission        evidence.Admission           `json:"admission"`
	Leaves           map[string]LeafEvidence      `json:"leaves"`
	Full             model.CompileOutput          `json:"full_emac_bound"`
	FeatureAware     model.CompileOutput          `json:"feature_aware_bound"`
	CurrentOracle    Oracle                       `json:"current_oracle"`
	EvaluationOracle Oracle                       `json:"evaluation_oracle"`
	ComponentGreen   map[string]*bool             `json:"component_green"`
	Decisions        map[controller.Method]string `json:"decisions"`
	EvidenceCutoff   time.Time                    `json:"evidence_cutoff"`
	EvidenceDigest   string                       `json:"evidence_scope"`
	IntegrityValid   bool                         `json:"integrity_valid"`
}

type Oracle struct {
	Interval statistics.Interval    `json:"interval"`
	Label    statistics.OracleLabel `json:"label"`
}

type counts struct {
	Intended, Attempted, Correct int
	Durations                    []float64
}

func Analyze(in Input) (Result, error) {
	if in.Look <= 0 || in.NMax < in.Look || in.Pipeline <= 0 || in.Pipeline > 1 || in.TargetWeight < 0 || in.TargetWeight > 1 {
		return Result{}, fmt.Errorf("invalid analysis input")
	}
	cal, err := loadCalibration(in.CalibrationPath)
	if err != nil {
		return Result{}, err
	}
	plan, err := loadPlan(in.PlanPath)
	if err != nil {
		return Result{}, err
	}
	requests, err := loadRequests(in.LedgerPath, in.Look)
	if err != nil {
		return Result{}, err
	}
	if len(requests) != in.Look {
		return Result{}, fmt.Errorf("look %d has %d measured ledger roots", in.Look, len(requests))
	}
	cutoff := requests[len(requests)-1].EndedAt
	if cutoff.IsZero() {
		return Result{}, fmt.Errorf("ledger lacks evidence timestamps")
	}
	looks := registeredLooks(in.NMax)
	alphaStar := .05 / (4 * float64(len(looks)) * 10)
	allHistograms, err := evidence.ReadHistograms(in.MetricsPath)
	if err != nil {
		return Result{}, err
	}
	projected, roots, conflict := project(requests, cal.JourneyDeadlineMS)
	leaves := map[string]LeafEvidence{}
	bands := map[string]model.Band{}
	componentGreen := map[string]*bool{}
	for branch, operations := range branchOperations {
		for _, operation := range operations {
			key := operation + "|" + branch
			c := projected[key]
			histogram, histogramErr := evidence.HistogramPrefix(allHistograms, operationName(operation), branch, in.Look)
			if histogramErr != nil || histogram.Count() != c.Correct {
				conflict = true
				componentGreen[key] = nil
				continue
			}
			band, bandErr := model.DKWBand(histogram.Bounds, histogram.BucketCounts[:len(histogram.Bounds)], c.Intended, alphaStar)
			if bandErr != nil {
				conflict = true
				componentGreen[key] = nil
				continue
			}
			bands[key] = band
			leaves[key] = LeafEvidence{Intended: c.Intended, CorrectAttempted: c.Correct, Band: band}
			componentGreen[key] = localGreen(c, histogram, cal.LocalDeadlinesMS[key])
		}
	}
	share, err := targetShare(plan, in.Look, in.TargetWeight, alphaStar)
	if err != nil {
		return Result{}, err
	}
	fullInput := model.CompileInput{
		Leaves:    bands,
		Candidate: names("candidate"), StableInternational: names("stable_international"), StableDomestic: names("stable_domestic"),
		Eligibility: .6, HLower: share.H.Lower, HUpper: share.H.Upper, Deadline: cal.JourneyDeadlineMS,
	}
	full, compileErr := model.CompileThreeCohort(fullInput)
	if compileErr != nil {
		conflict = true
	}
	feature, featureErr := featureBound(allHistograms, roots, in.Look, cal.JourneyDeadlineMS, share, len(looks))
	if featureErr != nil {
		conflict = true
	}
	admission, err := admissionAt(in.BeringDir, cutoff, in.Reconciled && !conflict)
	if err != nil {
		return Result{}, err
	}
	currentOracle, err := rootOracle(roots, in.CurrentWeight, cal.JourneyDeadlineMS, .05/(4*float64(len(looks))))
	if err != nil {
		return Result{}, err
	}
	oracleRequests, err := loadRequests(in.LedgerPath, min(1000, in.Look))
	if err != nil {
		return Result{}, err
	}
	_, oracleRoots, oracleConflict := project(oracleRequests, cal.JourneyDeadlineMS)
	evaluationOracle, err := rootOracle(oracleRoots, in.CurrentWeight, cal.JourneyDeadlineMS, .05)
	if err != nil {
		return Result{}, err
	}
	if oracleConflict {
		evaluationOracle = Oracle{Interval: statistics.Interval{Lower: 0, Upper: 1}, Label: statistics.Indeterminate}
	}
	decisions := map[controller.Method]string{
		controller.Full:         string(controller.FullEmaC(admission.Admitted, full.LowerAtDeadline, full.UpperAtDeadline, .95)),
		controller.FeatureAware: string(controller.FullEmaC(!conflict, feature.LowerAtDeadline, feature.UpperAtDeadline, .95)),
		controller.Reactive:     string(oracleDecision(currentOracle.Label)),
		controller.Local:        string(localDecision(componentGreen, positiveBranches(in.TargetWeight))),
		controller.Eager:        string(controller.FullEmaC(!conflict, full.LowerAtDeadline, full.UpperAtDeadline, .95)),
	}
	return Result{
		Schema: "emac.stage-analysis/v1", RunID: plan.RunID, StageID: plan.StageID, Pipeline: in.Pipeline,
		CurrentWeight: in.CurrentWeight, TargetWeight: in.TargetWeight, Look: in.Look, AlphaStar: alphaStar,
		TargetShare: share, Admission: admission, Leaves: leaves, Full: full, FeatureAware: feature,
		CurrentOracle: currentOracle, EvaluationOracle: evaluationOracle, ComponentGreen: componentGreen, Decisions: decisions,
		EvidenceCutoff: cutoff, EvidenceDigest: fmt.Sprintf("first-%d-measured-roots", in.Look), IntegrityValid: !conflict,
	}, nil
}

func loadCalibration(path string) (calibration.Result, error) {
	var result calibration.Result
	b, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(b, &result)
	return result, err
}

func loadPlan(path string) (experiment.StagePlan, error) {
	var plan experiment.StagePlan
	b, err := os.ReadFile(path)
	if err != nil {
		return plan, err
	}
	err = json.Unmarshal(b, &plan)
	return plan, err
}

func loadRequests(path string, look int) ([]ledger.Request, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	byIndex := map[int]ledger.Request{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var request ledger.Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return nil, err
		}
		if request.Phase != "measured" || request.EvidenceIndex < 0 || request.EvidenceIndex >= look {
			continue
		}
		if _, duplicate := byIndex[request.EvidenceIndex]; duplicate {
			return nil, fmt.Errorf("duplicate evidence index %d", request.EvidenceIndex)
		}
		byIndex[request.EvidenceIndex] = request
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := make([]ledger.Request, 0, len(byIndex))
	for i := 0; i < look; i++ {
		request, ok := byIndex[i]
		if !ok {
			continue
		}
		out = append(out, request)
	}
	return out, nil
}

func project(requests []ledger.Request, deadline float64) (map[string]counts, map[string]counts, bool) {
	leaves, roots := map[string]counts{}, map[string]counts{}
	conflict := false
	for _, request := range requests {
		observations, _, err := ledger.ValidateAndProject(request, time.Microsecond)
		if err != nil {
			conflict = true
			continue
		}
		root := roots[request.Branch]
		root.Intended++
		root.Attempted++
		if request.RootCorrect {
			root.Correct++
			root.Durations = append(root.Durations, float64(request.Root)/float64(time.Millisecond))
		}
		roots[request.Branch] = root
		for _, observation := range observations {
			key := observation.Operation + "|" + request.Branch
			value := leaves[key]
			value.Intended++
			if observation.Attempted {
				value.Attempted++
			}
			if observation.Correct && !math.IsInf(observation.Duration, 1) {
				value.Correct++
				value.Durations = append(value.Durations, observation.Duration)
			}
			leaves[key] = value
		}
	}
	_ = deadline
	return leaves, roots, conflict
}

func operationName(operation string) string {
	return operation
}

func registeredLooks(nMax int) []int {
	set := map[int]bool{}
	for _, n := range []int{1000, 2000, 4000, 8000, nMax} {
		if n >= 1000 && n <= nMax {
			set[n] = true
		}
	}
	result := make([]int, 0, len(set))
	for n := range set {
		result = append(result, n)
	}
	sort.Ints(result)
	return result
}

func names(branch string) []string {
	result := make([]string, len(branchOperations[branch]))
	for i, operation := range branchOperations[branch] {
		result[i] = operation + "|" + branch
	}
	return result
}

func targetShare(plan experiment.StagePlan, look int, target, alpha float64) (statistics.TargetShare, error) {
	eligible, assigned := 0, 0
	for _, request := range plan.Requests {
		if request.Phase != experiment.PhaseMeasured || request.EvidenceIndex < 0 || request.EvidenceIndex >= look || !request.International {
			continue
		}
		eligible++
		if request.Bucket < target {
			assigned++
		}
	}
	return statistics.CounterfactualShare(assigned, eligible, .6, alpha)
}

func admissionAt(dir string, cutoff time.Time, reconciled bool) (evidence.Admission, error) {
	windows, err := evidence.LoadArchive(dir)
	if err != nil {
		return evidence.Admission{}, err
	}
	selected := make([]evidence.ProjectionView, 0, len(windows))
	for _, window := range windows {
		if window.Snapshot == nil {
			continue
		}
		end, parseErr := time.Parse(time.RFC3339Nano, window.Snapshot.WindowEnd)
		if parseErr != nil {
			return evidence.Admission{}, parseErr
		}
		if !end.After(cutoff) {
			selected = append(selected, window)
		}
	}
	if len(selected) == 0 {
		return evidence.Admission{Reasons: []string{"no complete Bering window before evidence cutoff"}}, nil
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Observation < selected[j].Observation })
	stable, err := evidence.LoadStableArchive(dir, selected[len(selected)-1].Observation)
	if err != nil {
		return evidence.Admission{}, err
	}
	required := []evidence.RequiredEdge{{From: "checkout-policy", To: "cart", Operation: "GetCart"}, {From: "checkout-policy", To: "currency", Operation: "GetSupportedCurrencies"}, {From: "checkout-policy", To: "shipping", Operation: "POST"}, {From: "checkout-policy", To: "frontend", Operation: "POST"}}
	return evidence.Admit(selected, stable, required, 10, !reconciled), nil
}

func featureBound(histograms map[evidence.HistogramKey]evidence.Histogram, roots map[string]counts, look int, deadline float64, share statistics.TargetShare, k int) (model.CompileOutput, error) {
	alpha := .05 / (4 * float64(k) * 4)
	bands := make([]model.Band, 0, 3)
	for _, branch := range []string{"candidate", "stable_international", "stable_domestic"} {
		histogram, err := evidence.HistogramPrefix(histograms, "POST /api/checkout", branch, look)
		if err != nil || histogram.Count() != roots[branch].Correct {
			return model.CompileOutput{}, fmt.Errorf("root histogram conflict for %s", branch)
		}
		band, err := model.DKWBand(histogram.Bounds, histogram.BucketCounts[:len(histogram.Bounds)], roots[branch].Intended, alpha)
		if err != nil {
			return model.CompileOutput{}, err
		}
		bands = append(bands, band)
	}
	journey, err := model.ThreeCohort(bands[0], bands[1], bands[2], .6, share.H.Lower, share.H.Upper)
	if err != nil {
		return model.CompileOutput{}, err
	}
	lower, upper := journey.At(deadline)
	return model.CompileOutput{Journey: journey, Deadline: deadline, LowerAtDeadline: lower, UpperAtDeadline: upper}, nil
}

func rootOracle(roots map[string]counts, current, deadline, alpha float64) (Oracle, error) {
	masses := map[string]float64{"candidate": .6 * current, "stable_international": .6 * (1 - current), "stable_domestic": .4}
	cohorts := make([]statistics.CohortCount, 0, 3)
	for _, branch := range []string{"candidate", "stable_international", "stable_domestic"} {
		successes := 0
		for _, duration := range roots[branch].Durations {
			if duration <= deadline {
				successes++
			}
		}
		cohorts = append(cohorts, statistics.CohortCount{Name: branch, Successes: successes, Trials: roots[branch].Intended, DesignMass: masses[branch]})
	}
	interval, label, err := statistics.StratifiedOracle(cohorts, alpha, .95)
	return Oracle{Interval: interval, Label: label}, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func localGreen(c counts, histogram evidence.Histogram, deadline float64) *bool {
	if c.Attempted == 0 || c.Correct == 0 || deadline <= 0 {
		return nil
	}
	success := float64(c.Correct) / float64(c.Attempted)
	target := int(math.Ceil(.95 * float64(c.Correct)))
	cumulative := 0
	p95 := math.Inf(1)
	for i, n := range histogram.BucketCounts[:len(histogram.Bounds)] {
		cumulative += n
		if cumulative >= target {
			p95 = histogram.Bounds[i]
			break
		}
	}
	green := success >= .99 && p95 <= deadline
	return &green
}

func positiveBranches(target float64) map[string]bool {
	return map[string]bool{"candidate": target > 0, "stable_international": target < 1, "stable_domestic": true}
}

func localDecision(green map[string]*bool, positive map[string]bool) controller.Decision {
	decision := controller.Pass
	for key, value := range green {
		branch := branchFromKey(key)
		if !positive[branch] {
			continue
		}
		if value == nil {
			return controller.Review
		}
		if !*value {
			decision = controller.Block
		}
	}
	return decision
}

func branchFromKey(key string) string {
	for branch := range branchOperations {
		if len(key) > len(branch) && key[len(key)-len(branch):] == branch {
			return branch
		}
	}
	return ""
}

func oracleDecision(label statistics.OracleLabel) controller.Decision {
	switch label {
	case statistics.Safe:
		return controller.Pass
	case statistics.Unsafe:
		return controller.Block
	default:
		return controller.Review
	}
}
