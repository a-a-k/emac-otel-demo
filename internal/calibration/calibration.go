package calibration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/ledger"
)

type Result struct {
	Schema            string              `json:"schema"`
	JourneyDeadlineMS float64             `json:"journey_deadline_ms"`
	LocalDeadlinesMS  map[string]float64  `json:"local_deadlines_ms"`
	HistogramGridMS   []float64           `json:"histogram_grid_ms"`
	StableRuns        int                 `json:"stable_runs"`
	CandidateRuns     int                 `json:"candidate_runs"`
	Baselines         map[string]Baseline `json:"manipulation_baselines"`
}

type Baseline struct {
	SuccessRate float64 `json:"success_rate"`
	P95MS       float64 `json:"correct_attempted_p95_ms"`
}

type baselineAccumulator struct {
	Attempted, Correct int
	MaximumRunP95      float64
}

// Build applies only the registered mechanical calibration rule. Callers are
// responsible for supplying three weight-0 and three all-eligible candidate
// ledgers that already passed integrity and capacity checks.
func Build(stablePaths, candidatePaths []string) (Result, error) {
	if len(stablePaths) != 3 || len(candidatePaths) != 3 {
		return Result{}, fmt.Errorf("calibration requires exactly three stable and three candidate ledgers")
	}
	result := Result{Schema: "emac.calibration/v1", LocalDeadlinesMS: map[string]float64{}, Baselines: map[string]Baseline{}, StableRuns: len(stablePaths), CandidateRuns: len(candidatePaths)}
	maxRootP99 := 0.0
	maxOperationP95 := map[string]float64{}
	baselineTotals := map[string]*baselineAccumulator{}
	for _, path := range stablePaths {
		roots, operations, err := readLedger(path, false)
		if err != nil {
			return Result{}, err
		}
		maxRootP99 = math.Max(maxRootP99, quantile(roots, .99))
		mergeRunQuantiles(maxOperationP95, operations)
		if err := accumulateBaselines(path, baselineTotals); err != nil {
			return Result{}, err
		}
	}
	for _, path := range candidatePaths {
		_, operations, err := readLedger(path, true)
		if err != nil {
			return Result{}, err
		}
		mergeRunQuantiles(maxOperationP95, operations)
		if err := accumulateBaselines(path, baselineTotals); err != nil {
			return Result{}, err
		}
	}
	required := []string{
		"policy.residual|candidate", "CartService/GetCart|candidate", "CurrencyService/GetSupportedCurrencies|candidate", "Shipping/POST get-quote|candidate", "Frontend/POST api/checkout|candidate",
		"policy.residual|stable_international", "Frontend/POST api/checkout|stable_international",
		"policy.residual|stable_domestic", "Frontend/POST api/checkout|stable_domestic",
	}
	for _, operation := range required {
		if maxOperationP95[operation] <= 0 {
			return Result{}, fmt.Errorf("missing correct calibration observations for %s", operation)
		}
	}
	if maxRootP99 <= 0 {
		return Result{}, fmt.Errorf("no correct weight-0 policy-root durations")
	}
	result.JourneyDeadlineMS = roundUp10(1.10 * maxRootP99)
	for operation, p95 := range maxOperationP95 {
		result.LocalDeadlinesMS[operation] = roundUp10(1.20 * p95)
	}
	for operation, total := range baselineTotals {
		if total.Attempted == 0 || total.Correct == 0 {
			return Result{}, fmt.Errorf("invalid manipulation baseline for %s", operation)
		}
		result.Baselines[operation] = Baseline{SuccessRate: float64(total.Correct) / float64(total.Attempted), P95MS: total.MaximumRunP95}
	}
	result.HistogramGridMS = histogramGrid(result.JourneyDeadlineMS)
	return result, nil
}

func accumulateBaselines(path string, totals map[string]*baselineAccumulator) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	perRun := map[string][]float64{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var request ledger.Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return err
		}
		if request.Phase != "measured" {
			continue
		}
		_, residual, err := ledger.ValidateAndProject(request, time.Microsecond)
		if err != nil {
			return err
		}
		residualKey := "policy.residual|" + request.Branch
		if totals[residualKey] == nil {
			totals[residualKey] = &baselineAccumulator{}
		}
		totals[residualKey].Attempted++
		if request.RootCorrect {
			totals[residualKey].Correct++
			perRun[residualKey] = append(perRun[residualKey], durationMS(residual))
		}
		for _, call := range request.Calls {
			if !call.Attempted {
				continue
			}
			key := call.Operation + "|" + request.Branch
			if totals[key] == nil {
				totals[key] = &baselineAccumulator{}
			}
			totals[key].Attempted++
			if call.Correct {
				totals[key].Correct++
				perRun[key] = append(perRun[key], durationMS(call.Duration))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for key, durations := range perRun {
		totals[key].MaximumRunP95 = math.Max(totals[key].MaximumRunP95, quantile(durations, .95))
	}
	return nil
}

func readLedger(path string, candidateOnly bool) ([]float64, map[string][]float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	var roots []float64
	operations := map[string][]float64{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var request ledger.Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return nil, nil, err
		}
		if request.Phase != "measured" {
			continue
		}
		if candidateOnly && request.Branch != "candidate" {
			return nil, nil, fmt.Errorf("candidate characterization contains branch %s", request.Branch)
		}
		if request.RootCorrect {
			roots = append(roots, durationMS(request.Root))
		}
		_, residual, err := ledger.ValidateAndProject(request, time.Microsecond)
		if err != nil {
			return nil, nil, err
		}
		if request.RootCorrect {
			key := "policy.residual|" + request.Branch
			operations[key] = append(operations[key], durationMS(residual))
		}
		for _, call := range request.Calls {
			if call.Attempted && call.Correct {
				key := call.Operation + "|" + request.Branch
				operations[key] = append(operations[key], durationMS(call.Duration))
			}
		}
	}
	return roots, operations, scanner.Err()
}

func mergeRunQuantiles(maxima map[string]float64, operations map[string][]float64) {
	for operation, durations := range operations {
		maxima[operation] = math.Max(maxima[operation], quantile(durations, .95))
	}
}

func quantile(values []float64, probability float64) float64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	index := int(math.Ceil(probability*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}

func roundUp10(value float64) float64        { return math.Ceil(value/10) * 10 }
func durationMS(value time.Duration) float64 { return float64(value) / float64(time.Millisecond) }

func histogramGrid(deadline float64) []float64 {
	grid := make([]float64, 0, 48)
	for i := 1; i <= 32; i++ {
		grid = append(grid, deadline*float64(i)/32)
	}
	for i := 1; i <= 16; i++ {
		grid = append(grid, deadline+deadline*float64(i)/16)
	}
	return grid
}
