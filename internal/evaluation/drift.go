package evaluation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/a-a-k/emac-otel-demo/internal/analysis"
	"github.com/a-a-k/emac-otel-demo/internal/controller"
	"github.com/a-a-k/emac-otel-demo/internal/evidence"
	"github.com/a-a-k/emac-otel-demo/internal/model"
	"github.com/a-a-k/emac-otel-demo/internal/statistics"
)

type DriftResult struct {
	Transition       string                 `json:"transition"`
	Leaf             string                 `json:"leaf"`
	Applicable       bool                   `json:"applicable"`
	DKW              statistics.DriftResult `json:"two_sample_dkw"`
	P95Ratio         float64                `json:"p95_ratio"`
	P95RatioLower    float64                `json:"p95_ratio_lower"`
	P95RatioUpper    float64                `json:"p95_ratio_upper"`
	P95DriftDetected bool                   `json:"p95_drift_detected"`
	DriftDetected    bool                   `json:"drift_detected"`
	DecisionChanged  bool                   `json:"decision_changed"`
	Reason           string                 `json:"reason,omitempty"`
}

func compareTransition(current, target analysis.Result, currentWeight, targetWeight int) ([]DriftResult, bool, error) {
	decisionChanged, err := recompileChanged(current, target)
	if err != nil {
		return nil, false, err
	}
	alpha := .05 / (4 * 9 * 2)
	keys := make([]string, 0, len(current.Leaves))
	for key := range current.Leaves {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := make([]DriftResult, 0, len(keys))
	for index, key := range keys {
		value := DriftResult{Transition: fmt.Sprintf("%d->%d", currentWeight, targetWeight), Leaf: key, DecisionChanged: decisionChanged}
		before := current.Leaves[key]
		after, exists := target.Leaves[key]
		if !exists || before.Intended == 0 || after.Intended == 0 || before.CorrectAttempted == 0 || after.CorrectAttempted == 0 {
			value.Reason = "cohort has zero target mass or insufficient correct attempts"
			results = append(results, value)
			continue
		}
		value.Applicable = true
		distance, err := histogramDistance(before, after)
		if err != nil {
			return nil, false, err
		}
		value.DKW, err = statistics.TwoSampleDKW(distance, before.Intended, after.Intended, alpha, decisionChanged)
		if err != nil {
			return nil, false, err
		}
		value.P95Ratio, value.P95RatioLower, value.P95RatioUpper = bootstrapP95Ratio(before.Histogram, after.Histogram, alpha, int64(currentWeight*100000+targetWeight*1000+index))
		value.P95DriftDetected = value.P95RatioUpper < .90 || value.P95RatioLower > 1.10
		value.DriftDetected = value.DKW.Detected || value.P95DriftDetected
		results = append(results, value)
	}
	return results, decisionChanged, nil
}

func histogramDistance(a, b analysis.LeafEvidence) (float64, error) {
	if len(a.Histogram.Bounds) != len(b.Histogram.Bounds) {
		return 0, fmt.Errorf("histogram grid changed")
	}
	ca, cb, distance := 0, 0, 0.0
	for i := range a.Histogram.Bounds {
		if math.Abs(a.Histogram.Bounds[i]-b.Histogram.Bounds[i]) > 1e-9 {
			return 0, fmt.Errorf("histogram grid changed")
		}
		ca += a.Histogram.BucketCounts[i]
		cb += b.Histogram.BucketCounts[i]
		distance = math.Max(distance, math.Abs(float64(ca)/float64(a.Intended)-float64(cb)/float64(b.Intended)))
	}
	return distance, nil
}

func bootstrapP95Ratio(before, after evidence.Histogram, alpha float64, seed int64) (float64, float64, float64) {
	pointBefore := histogramP95(before, before.BucketCounts)
	pointAfter := histogramP95(after, after.BucketCounts)
	point := safeRatio(pointAfter, pointBefore)
	rng := rand.New(rand.NewSource(seed))
	const replicates = 10000
	ratios := make([]float64, 0, replicates)
	for i := 0; i < replicates; i++ {
		beforeCounts := poissonHistogram(rng, before.BucketCounts)
		afterCounts := poissonHistogram(rng, after.BucketCounts)
		ratio := safeRatio(histogramP95(after, afterCounts), histogramP95(before, beforeCounts))
		if !math.IsNaN(ratio) && !math.IsInf(ratio, 0) {
			ratios = append(ratios, ratio)
		}
	}
	if len(ratios) == 0 {
		return point, 0, math.Inf(1)
	}
	sort.Float64s(ratios)
	lowerIndex := int(math.Floor((alpha / 2) * float64(len(ratios)-1)))
	upperIndex := int(math.Ceil((1 - alpha/2) * float64(len(ratios)-1)))
	return point, ratios[lowerIndex], ratios[upperIndex]
}

func poissonHistogram(rng *rand.Rand, counts []int) []int {
	result := make([]int, len(counts))
	for i, count := range counts {
		if count <= 0 {
			continue
		}
		if count > 30 {
			value := int(math.Round(rng.NormFloat64()*math.Sqrt(float64(count)) + float64(count)))
			if value > 0 {
				result[i] = value
			}
			continue
		}
		limit, product, k := math.Exp(-float64(count)), 1.0, 0
		for product > limit {
			k++
			product *= rng.Float64()
		}
		result[i] = k - 1
	}
	return result
}

func histogramP95(histogram evidence.Histogram, counts []int) float64 {
	total := 0
	for _, count := range counts {
		total += count
	}
	if total == 0 {
		return math.NaN()
	}
	target := int(math.Ceil(.95 * float64(total)))
	cumulative := 0
	for i, count := range counts {
		cumulative += count
		if cumulative >= target {
			if i < len(histogram.Bounds) {
				return histogram.Bounds[i]
			}
			return math.Inf(1)
		}
	}
	return math.Inf(1)
}

func safeRatio(numerator, denominator float64) float64 {
	if denominator == 0 || math.IsNaN(numerator) || math.IsNaN(denominator) {
		return math.NaN()
	}
	return numerator / denominator
}

func recompileChanged(current, target analysis.Result) (bool, error) {
	leaves := map[string]model.Band{}
	for key, before := range current.Leaves {
		if after, exists := target.Leaves[key]; exists {
			leaves[key] = after.Band
		} else {
			leaves[key] = before.Band
		}
	}
	input := model.CompileInput{
		Leaves:              leaves,
		Candidate:           []string{"policy.residual|candidate", "CartService/GetCart|candidate", "CurrencyService/GetSupportedCurrencies|candidate", "Shipping/POST get-quote|candidate", "Frontend/POST api/checkout|candidate"},
		StableInternational: []string{"policy.residual|stable_international", "Frontend/POST api/checkout|stable_international"},
		StableDomestic:      []string{"policy.residual|stable_domestic", "Frontend/POST api/checkout|stable_domestic"},
		Eligibility:         .6, HLower: current.TargetShare.H.Lower, HUpper: current.TargetShare.H.Upper, Deadline: current.Full.Deadline,
	}
	recompiled, err := model.CompileThreeCohort(input)
	if err != nil {
		return false, err
	}
	original := controller.FullEmaC(current.Admission.Admitted, current.Full.LowerAtDeadline, current.Full.UpperAtDeadline, .95)
	updated := controller.FullEmaC(current.Admission.Admitted, recompiled.LowerAtDeadline, recompiled.UpperAtDeadline, .95)
	return original != updated, nil
}
