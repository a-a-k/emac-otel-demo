package statistics

import "fmt"

type CohortCount struct {
	Name       string  `json:"name"`
	Successes  int     `json:"successes"`
	Trials     int     `json:"trials"`
	DesignMass float64 `json:"design_mass"`
}
type OracleLabel string

const (
	Safe          OracleLabel = "SAFE"
	Unsafe        OracleLabel = "UNSAFE"
	Indeterminate OracleLabel = "INDETERMINATE"
)

// StratifiedOracle constructs simultaneous cohort-specific CP intervals and
// then bounds the registered design-conditional mixture.
func StratifiedOracle(counts []CohortCount, alpha, target float64) (Interval, OracleLabel, error) {
	if len(counts) == 0 {
		return Interval{}, "", fmt.Errorf("no cohorts")
	}
	var mix Interval
	for _, c := range counts {
		if c.DesignMass < 0 {
			return Interval{}, "", fmt.Errorf("negative design mass")
		}
		ci, err := ClopperPearson(c.Successes, c.Trials, alpha/float64(len(counts)))
		if err != nil {
			return Interval{}, "", err
		}
		mass := c.DesignMass
		if c.Trials == 0 {
			mass = 0
		}
		mix.Lower += mass * ci.Lower
		mix.Upper += mass * ci.Upper
	}
	label := Indeterminate
	if mix.Lower >= target {
		label = Safe
	} else if mix.Upper < target {
		label = Unsafe
	}
	return mix, label, nil
}
