package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/a-a-k/emac-otel-demo/internal/analysis"
	"github.com/a-a-k/emac-otel-demo/internal/controller"
)

type ReplayResult struct {
	Schema         string                  `json:"schema"`
	NMax           int                     `json:"n_max"`
	CanaryLabel    string                  `json:"canary_label"`
	OracleLabels   []string                `json:"oracle_labels"`
	Methods        map[string]MethodResult `json:"methods"`
	Identifying    bool                    `json:"identifying"`
	IdentifyReason []string                `json:"identifiability_reasons,omitempty"`
	Drift          []DriftResult           `json:"transportability"`
}

type MethodResult struct {
	Decisions      []controller.Decision        `json:"decisions"`
	DecisionLooks  []int                        `json:"decision_looks"`
	Bounds         [][2]float64                 `json:"bounds_at_decision"`
	Outcome        controller.TrajectoryOutcome `json:"outcome"`
	StoppingTarget int                          `json:"stopping_target,omitempty"`
}

type methodSpec struct {
	name     string
	method   controller.Method
	pipeline string
}

var replayMethods = []methodSpec{
	{"FullEmaC", controller.Full, "100"},
	{"FullEmaC-shadow-25", controller.Full, "25"},
	{"FullEmaC-shadow-05", controller.Full, "05"},
	{"Local", controller.Local, "100"},
	{"Reactive", controller.Reactive, "100"},
	{"FeatureAware", controller.FeatureAware, "100"},
	{"Eager", controller.Eager, "100"},
}

func Replay(root string, nMax int) (ReplayResult, error) {
	if nMax < 1000 {
		return ReplayResult{}, fmt.Errorf("n_max must be at least 1000")
	}
	looks := replayLooks(nMax)
	weights := []int{10, 25, 50, 75, 100}
	final := map[int]analysis.Result{}
	for _, weight := range weights {
		value, err := loadStageAnalysis(root, weight, nMax, nMax, "100")
		if err != nil {
			return ReplayResult{}, err
		}
		final[weight] = value
	}
	result := ReplayResult{Schema: "emac.offline-replay/v1", NMax: nMax, CanaryLabel: string(final[10].EvaluationOracle.Label), Methods: map[string]MethodResult{}}
	for _, target := range weights[1:] {
		result.OracleLabels = append(result.OracleLabels, string(final[target].EvaluationOracle.Label))
	}
	for _, spec := range replayMethods {
		machine := controller.Machine{Method: spec.method}
		methodResult := MethodResult{}
		for transition, current := range weights[:len(weights)-1] {
			target := weights[transition+1]
			stageDecided := false
			for _, look := range looks {
				value, err := loadStageAnalysis(root, current, nMax, look, spec.pipeline)
				if err != nil {
					return ReplayResult{}, err
				}
				input := controller.StageInput{
					Admitted: value.Admission.Admitted, FeatureEvidence: value.IntegrityValid, FinalLook: look == nMax,
					ComponentGreen: value.ComponentGreen, CurrentOracle: string(value.CurrentOracle.Label),
					FullLower: value.Full.LowerAtDeadline, FullUpper: value.Full.UpperAtDeadline,
					FeatureLower: value.FeatureAware.LowerAtDeadline, FeatureUpper: value.FeatureAware.UpperAtDeadline, Target: .95,
				}
				decision, consumed := machine.Step(input)
				if !consumed {
					stageDecided = true
					break
				}
				if decision == controller.Review && look < nMax {
					continue
				}
				methodResult.Decisions = append(methodResult.Decisions, decision)
				methodResult.DecisionLooks = append(methodResult.DecisionLooks, look)
				if spec.method == controller.FeatureAware {
					methodResult.Bounds = append(methodResult.Bounds, [2]float64{value.FeatureAware.LowerAtDeadline, value.FeatureAware.UpperAtDeadline})
				} else {
					methodResult.Bounds = append(methodResult.Bounds, [2]float64{value.Full.LowerAtDeadline, value.Full.UpperAtDeadline})
				}
				stageDecided = true
				if decision == controller.Block || decision == controller.Review {
					methodResult.StoppingTarget = target
				}
				break
			}
			if !stageDecided || machine.Terminal {
				break
			}
		}
		methodResult.Outcome = controller.Outcomes(result.OracleLabels, methodResult.Decisions)
		result.Methods[spec.name] = methodResult
	}
	for transition, currentWeight := range weights[:len(weights)-1] {
		targetWeight := weights[transition+1]
		if len(final[currentWeight].Leaves) == 0 || len(final[targetWeight].Leaves) == 0 {
			continue
		}
		drift, _, err := compareTransition(final[currentWeight], final[targetWeight], currentWeight, targetWeight)
		if err != nil {
			return ReplayResult{}, err
		}
		result.Drift = append(result.Drift, drift...)
	}
	result.Identifying, result.IdentifyReason = identifiability(result, final)
	return result, nil
}

func loadStageAnalysis(root string, weight, cap, look int, pipeline string) (analysis.Result, error) {
	path := filepath.Join(root, fmt.Sprintf("weight-%03d", weight), "analysis", fmt.Sprintf("analysis-cap-%05d-look-%05d-p%s.json", cap, look, pipeline))
	var result analysis.Result
	b, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("load %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return result, err
	}
	return result, nil
}

func replayLooks(nMax int) []int {
	set := map[int]bool{}
	for _, look := range []int{1000, 2000, 4000, 8000, nMax} {
		if look <= nMax {
			set[look] = true
		}
	}
	result := make([]int, 0, len(set))
	for look := range set {
		result = append(result, look)
	}
	sort.Ints(result)
	return result
}

func identifiability(result ReplayResult, stages map[int]analysis.Result) (bool, []string) {
	var reasons []string
	if result.CanaryLabel != "SAFE" {
		reasons = append(reasons, "10% canary is not SAFE")
	}
	firstUnsafe := -1
	for i, label := range result.OracleLabels {
		if label == "UNSAFE" {
			firstUnsafe = i
			break
		}
	}
	if firstUnsafe < 1 || firstUnsafe > 2 {
		reasons = append(reasons, "controlled trajectory lacks a SAFE target in 25/50/75 followed by a higher UNSAFE target")
	} else {
		for i := 0; i < firstUnsafe; i++ {
			if result.OracleLabels[i] != "SAFE" {
				reasons = append(reasons, "oracle SAFE prefix is interrupted")
			}
		}
	}
	full := result.Methods["FullEmaC"]
	if firstUnsafe >= 0 {
		if len(full.Decisions) <= firstUnsafe {
			reasons = append(reasons, "Full EmaC stopped before first UNSAFE target")
		} else {
			for i := 0; i < firstUnsafe; i++ {
				if full.Decisions[i] != controller.Pass {
					reasons = append(reasons, "Full EmaC did not PASS complete SAFE prefix")
				}
			}
			if full.Decisions[firstUnsafe] != controller.Block {
				reasons = append(reasons, "Full EmaC did not BLOCK first UNSAFE target")
			}
		}
	}
	for weight, stage := range stages {
		if !stage.Manipulation.Valid {
			reasons = append(reasons, fmt.Sprintf("weight %d is manipulation-invalid", weight))
		}
		for key, value := range stage.ComponentGreen {
			if weight == 100 && hasSuffix(key, "stable_international") {
				continue
			}
			if value == nil || !*value {
				reasons = append(reasons, fmt.Sprintf("weight %d is not component-green", weight))
				break
			}
		}
	}
	return len(reasons) == 0, reasons
}

func hasSuffix(value, suffix string) bool {
	return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
}
