package validation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/a-a-k/emac-otel-demo/internal/model"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/optimize/convex/lp"
)

type RQ1Result struct {
	Schema          string         `json:"schema"`
	Seed            int64          `json:"seed"`
	Cases           int            `json:"cases"`
	CasesByOperator map[string]int `json:"cases_by_operator"`
	MaximumWidth    float64        `json:"maximum_bound_width"`
	Violations      int            `json:"violations"`
	LPCases         int            `json:"lp_cases_2_to_4_leaves"`
	LPPoints        int            `json:"lp_points"`
	MaximumLPGap    float64        `json:"maximum_width_gap_to_lp"`
}

// ValidateRQ1 executes seeded finite-support soundness cases. Every generated
// exact joint CDF must be contained by the corresponding model-free bound.
func ValidateRQ1(seed int64, cases int) (RQ1Result, error) {
	if cases <= 0 {
		return RQ1Result{}, fmt.Errorf("cases must be positive")
	}
	rng := rand.New(rand.NewSource(seed))
	result := RQ1Result{Schema: "emac.rq1-validation/v1", Seed: seed, Cases: cases, CasesByOperator: map[string]int{}}
	operators := []string{"Series", "Cond", "Parallel", "Race", "Timeout"}
	for caseIndex := 0; caseIndex < cases; caseIndex++ {
		operator := operators[caseIndex%len(operators)]
		result.CasesByOperator[operator]++
		atoms := 2 + rng.Intn(7)
		leafCount := 2 + rng.Intn(4)
		values := randomJoint(rng, atoms, leafCount)
		var bound model.Band
		var actual []weightedValue
		var err error
		switch operator {
		case "Series":
			bound, err = model.Series(exactMarginals(values)...)
			actual = evaluateJoint(values, func(row []float64) float64 {
				total := 0.0
				for _, value := range row {
					if math.IsInf(value, 1) {
						return math.Inf(1)
					}
					total += value
				}
				return total
			})
		case "Parallel":
			bound, err = model.Parallel(exactMarginals(values)...)
			actual = evaluateJoint(values, func(row []float64) float64 {
				maximum := 0.0
				for _, value := range row {
					maximum = math.Max(maximum, value)
				}
				return maximum
			})
		case "Race":
			bound, err = model.Race(exactMarginals(values)...)
			actual = evaluateJoint(values, func(row []float64) float64 {
				minimum := math.Inf(1)
				for _, value := range row {
					minimum = math.Min(minimum, value)
				}
				return minimum
			})
		case "Timeout":
			deadline := float64(1 + rng.Intn(5))
			bound, err = model.Timeout(exactMarginal(column(values, 0)), deadline)
			actual = evaluateJoint(values, func(row []float64) float64 {
				if row[0] > deadline {
					return math.Inf(1)
				}
				return row[0]
			})
		case "Cond":
			qNumerator := 1 + rng.Intn(atoms-1)
			q := float64(qNumerator) / float64(atoms)
			a := exactMarginal(column(values, 0))
			b := exactMarginal(column(values, 1))
			bound, err = model.Cond(a, b, q, q)
			actual = mixture(column(values, 0), column(values, 1), q)
		}
		if err != nil {
			return result, fmt.Errorf("case %d %s: %w", caseIndex, operator, err)
		}
		violation, width := checkContainment(bound, actual)
		result.MaximumWidth = math.Max(result.MaximumWidth, width)
		if violation {
			result.Violations++
			return result, fmt.Errorf("soundness violation in case %d operator %s", caseIndex, operator)
		}
		if operator == "Series" && leafCount <= 4 && caseIndex < 1000 {
			points, gap, lpErr := compareSeriesLP(bound, values)
			if lpErr != nil {
				return result, fmt.Errorf("case %d LP comparison: %w", caseIndex, lpErr)
			}
			result.LPCases++
			result.LPPoints += points
			result.MaximumLPGap = math.Max(result.MaximumLPGap, gap)
		}
	}
	return result, nil
}

type marginalState struct {
	value, probability float64
}

func compareSeriesLP(bound model.Band, values [][]float64) (int, float64, error) {
	marginals := make([][]marginalState, len(values[0]))
	for leaf := range marginals {
		counts := map[float64]int{}
		for _, value := range column(values, leaf) {
			counts[value]++
		}
		for value, count := range counts {
			marginals[leaf] = append(marginals[leaf], marginalState{value: value, probability: float64(count) / float64(len(values))})
		}
		sort.Slice(marginals[leaf], func(i, j int) bool { return marginals[leaf][i].value < marginals[leaf][j].value })
	}
	maximumGap := 0.0
	for _, threshold := range bound.Grid {
		minimum, maximum, err := seriesLPInterval(marginals, threshold)
		if err != nil {
			return 0, 0, err
		}
		lower, upper := bound.At(threshold)
		if lower > minimum+1e-7 || upper < maximum-1e-7 {
			return 0, 0, fmt.Errorf("bound [%g,%g] excludes LP [%g,%g] at %g", lower, upper, minimum, maximum, threshold)
		}
		maximumGap = math.Max(maximumGap, (upper-lower)-(maximum-minimum))
	}
	return len(bound.Grid), maximumGap, nil
}

func seriesLPInterval(marginals [][]marginalState, threshold float64) (float64, float64, error) {
	var assignments [][]int
	var enumerate func(int, []int)
	enumerate = func(leaf int, prefix []int) {
		if leaf == len(marginals) {
			assignments = append(assignments, append([]int(nil), prefix...))
			return
		}
		for state := range marginals[leaf] {
			enumerate(leaf+1, append(prefix, state))
		}
	}
	enumerate(0, nil)
	rows := 1
	for _, marginal := range marginals {
		rows += len(marginal) - 1
	}
	a := mat.NewDense(rows, len(assignments), nil)
	b := make([]float64, rows)
	for column := range assignments {
		a.Set(0, column, 1)
	}
	b[0] = 1
	row := 1
	for leaf, marginal := range marginals {
		for state := 0; state < len(marginal)-1; state++ {
			for column, assignment := range assignments {
				if assignment[leaf] == state {
					a.Set(row, column, 1)
				}
			}
			b[row] = marginal[state].probability
			row++
		}
	}
	objective := make([]float64, len(assignments))
	for column, assignment := range assignments {
		total := 0.0
		for leaf, state := range assignment {
			total += marginals[leaf][state].value
		}
		if total <= threshold {
			objective[column] = 1
		}
	}
	minimum, _, err := lp.Simplex(objective, a, b, 1e-10, nil)
	if err != nil {
		return 0, 0, err
	}
	negative := make([]float64, len(objective))
	for i, value := range objective {
		negative[i] = -value
	}
	maximumNegative, _, err := lp.Simplex(negative, a, b, 1e-10, nil)
	if err != nil {
		return 0, 0, err
	}
	return minimum, -maximumNegative, nil
}

type weightedValue struct {
	Value, Weight float64
}

func randomJoint(rng *rand.Rand, atoms, leaves int) [][]float64 {
	values := make([][]float64, atoms)
	for atom := range values {
		values[atom] = make([]float64, leaves)
		for leaf := range values[atom] {
			value := float64(1 + rng.Intn(4))
			if rng.Intn(20) == 0 {
				value = math.Inf(1)
			}
			values[atom][leaf] = value
		}
	}
	for leaf := 0; leaf < leaves; leaf++ {
		values[0][leaf] = float64(1 + rng.Intn(4))
	}
	return values
}

func column(values [][]float64, index int) []float64 {
	result := make([]float64, len(values))
	for i := range values {
		result[i] = values[i][index]
	}
	return result
}

func exactMarginals(values [][]float64) []model.Band {
	result := make([]model.Band, len(values[0]))
	for leaf := range result {
		result[leaf] = exactMarginal(column(values, leaf))
	}
	return result
}

func exactMarginal(values []float64) model.Band {
	set := map[float64]bool{}
	for _, value := range values {
		if !math.IsInf(value, 1) {
			set[value] = true
		}
	}
	grid := make([]float64, 0, len(set))
	for value := range set {
		grid = append(grid, value)
	}
	sort.Float64s(grid)
	band := model.Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid))}
	for i, boundary := range grid {
		count := 0
		for _, value := range values {
			if value <= boundary {
				count++
			}
		}
		band.Lower[i] = float64(count) / float64(len(values))
		band.Upper[i] = band.Lower[i]
	}
	return band
}

func evaluateJoint(values [][]float64, operation func([]float64) float64) []weightedValue {
	result := make([]weightedValue, len(values))
	for i, row := range values {
		result[i] = weightedValue{Value: operation(row), Weight: 1 / float64(len(values))}
	}
	return result
}

func mixture(a, b []float64, q float64) []weightedValue {
	result := make([]weightedValue, 0, len(a)+len(b))
	for _, value := range a {
		result = append(result, weightedValue{Value: value, Weight: q / float64(len(a))})
	}
	for _, value := range b {
		result = append(result, weightedValue{Value: value, Weight: (1 - q) / float64(len(b))})
	}
	return result
}

func checkContainment(bound model.Band, actual []weightedValue) (bool, float64) {
	points := append([]float64(nil), bound.Grid...)
	for _, observation := range actual {
		if !math.IsInf(observation.Value, 1) {
			points = append(points, observation.Value)
		}
	}
	maximumWidth := 0.0
	for _, point := range points {
		cdf := 0.0
		for _, observation := range actual {
			if observation.Value <= point {
				cdf += observation.Weight
			}
		}
		lower, upper := bound.At(point)
		maximumWidth = math.Max(maximumWidth, upper-lower)
		if cdf < lower-1e-9 || cdf > upper+1e-9 {
			return true, maximumWidth
		}
	}
	return false, maximumWidth
}
