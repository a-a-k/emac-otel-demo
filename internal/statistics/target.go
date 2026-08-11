package statistics

import (
	"fmt"
	"math"
)

type TargetShare struct {
	H        Interval `json:"h"`
	Q        Interval `json:"q"`
	Eligible int      `json:"eligible"`
	Assigned int      `json:"assigned"`
}

func CounterfactualShare(assigned, eligible int, eligibilityMass, alpha float64) (TargetShare, error) {
	if eligibilityMass < 0 || eligibilityMass > 1 {
		return TargetShare{}, fmt.Errorf("eligibility mass outside [0,1]")
	}
	h, err := ClopperPearson(assigned, eligible, alpha)
	if err != nil {
		return TargetShare{}, err
	}
	return TargetShare{h, Interval{eligibilityMass * h.Lower, eligibilityMass * h.Upper}, eligible, assigned}, nil
}

type DriftResult struct {
	Distance, Critical        float64 `json:",omitempty"`
	Detected, DecisionChanged bool
}

// TwoSampleDKW is a conservative no-detected-drift check, not an equivalence
// certificate. decisionChanged is recorded orthogonally after recompilation.
func TwoSampleDKW(distance float64, n, m int, alpha float64, decisionChanged bool) (DriftResult, error) {
	if distance < 0 || distance > 1 || n <= 0 || m <= 0 || alpha <= 0 || alpha >= 1 {
		return DriftResult{}, fmt.Errorf("invalid drift input")
	}
	critical := math.Sqrt(-.5 * math.Log(alpha/2) * (float64(n+m) / float64(n*m)))
	return DriftResult{distance, critical, distance > critical, decisionChanged}, nil
}
