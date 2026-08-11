package evaluation

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/a-a-k/emac-otel-demo/internal/statistics"
)

type ConfirmatoryResult struct {
	Schema string                  `json:"schema"`
	Runs   int                     `json:"paired_runs"`
	Tests  []ConfirmatoryTest      `json:"tests"`
	Holm   []statistics.HolmResult `json:"holm_fwer_0_05"`
}

type ConfirmatoryTest struct {
	Name                  string  `json:"name"`
	Outcome               string  `json:"outcome"`
	Comparison            string  `json:"comparison"`
	FullTrueBaselineFalse int     `json:"full_true_baseline_false"`
	FullFalseBaselineTrue int     `json:"full_false_baseline_true"`
	P                     float64 `json:"two_sided_exact_mcnemar_p"`
}

func Confirmatory(paths []string) (ConfirmatoryResult, error) {
	if len(paths) != 40 {
		return ConfirmatoryResult{}, fmt.Errorf("confirmatory analysis requires exactly 40 paired runs, got %d", len(paths))
	}
	runs := make([]ReplayResult, len(paths))
	for i, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return ConfirmatoryResult{}, err
		}
		if err := json.Unmarshal(b, &runs[i]); err != nil {
			return ConfirmatoryResult{}, err
		}
	}
	result := ConfirmatoryResult{Schema: "emac.confirmatory/v1", Runs: len(runs)}
	pValues := map[string]float64{}
	for _, outcome := range []string{"A", "Z"} {
		for _, comparison := range []string{"Local", "Reactive", "FeatureAware"} {
			test := ConfirmatoryTest{Name: "FullEmaC_vs_" + comparison + "_on_" + outcome, Outcome: outcome, Comparison: comparison}
			for _, run := range runs {
				full, okFull := run.Methods["FullEmaC"]
				baseline, okBaseline := run.Methods[comparison]
				if !okFull || !okBaseline {
					return ConfirmatoryResult{}, fmt.Errorf("run lacks FullEmaC or %s", comparison)
				}
				fullValue, baselineValue := outcomeValue(full, outcome), outcomeValue(baseline, outcome)
				if fullValue && !baselineValue {
					test.FullTrueBaselineFalse++
				} else if !fullValue && baselineValue {
					test.FullFalseBaselineTrue++
				}
			}
			p, err := statistics.ExactMcNemarTwoSided(test.FullTrueBaselineFalse, test.FullFalseBaselineTrue)
			if err != nil {
				return ConfirmatoryResult{}, err
			}
			test.P = p
			result.Tests = append(result.Tests, test)
			pValues[test.Name] = p
		}
	}
	holm, err := statistics.Holm(pValues, .05)
	if err != nil {
		return ConfirmatoryResult{}, err
	}
	result.Holm = holm
	return result, nil
}

func outcomeValue(method MethodResult, outcome string) bool {
	if outcome == "A" {
		return method.Outcome.A
	}
	return method.Outcome.Z
}
