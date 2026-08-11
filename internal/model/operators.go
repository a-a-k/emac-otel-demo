package model

import (
	"fmt"
	"math"
	"sort"
)

// Cond returns an outer band for q*A + (1-q)*B, q in [qLower,qUpper].
func Cond(a, b Band, qLower, qUpper float64) (Band, error) {
	if err := a.Validate(); err != nil {
		return Band{}, err
	}
	if err := b.Validate(); err != nil {
		return Band{}, err
	}
	if qLower < 0 || qUpper > 1 || qLower > qUpper {
		return Band{}, fmt.Errorf("invalid condition interval")
	}
	grid := unionGrid(a, b)
	out := Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid)), IntervalCensored: a.IntervalCensored || b.IntervalCensored}
	for i, x := range grid {
		al, au := a.At(x)
		bl, bu := b.At(x)
		lower := func(q float64) float64 { return q*al + (1-q)*bl }
		upper := func(q float64) float64 { return q*au + (1-q)*bu }
		out.Lower[i] = math.Min(lower(qLower), lower(qUpper))
		out.Upper[i] = math.Max(upper(qLower), upper(qUpper))
	}
	return out, nil
}

// Parallel is the max latency of operations that must all complete correctly.
// Fréchet bounds require no dependence assumption.
func Parallel(leaves ...Band) (Band, error) {
	if len(leaves) == 0 {
		return Band{}, fmt.Errorf("empty parallel")
	}
	grid, censored, err := operatorGrid(leaves)
	if err != nil {
		return Band{}, err
	}
	out := Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid)), IntervalCensored: censored}
	for i, x := range grid {
		lowerSum, upperMin := 0.0, 1.0
		for _, leaf := range leaves {
			lower, upper := leaf.At(x)
			lowerSum += lower
			upperMin = math.Min(upperMin, upper)
		}
		out.Lower[i] = math.Max(0, lowerSum-float64(len(leaves)-1))
		out.Upper[i] = upperMin
	}
	return out, nil
}

// Race is the min latency of alternatives where any correct completion wins.
func Race(leaves ...Band) (Band, error) {
	if len(leaves) == 0 {
		return Band{}, fmt.Errorf("empty race")
	}
	grid, censored, err := operatorGrid(leaves)
	if err != nil {
		return Band{}, err
	}
	out := Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid)), IntervalCensored: censored}
	for i, x := range grid {
		lowerMax, upperSum := 0.0, 0.0
		for _, leaf := range leaves {
			lower, upper := leaf.At(x)
			lowerMax = math.Max(lowerMax, lower)
			upperSum += upper
		}
		out.Lower[i] = lowerMax
		out.Upper[i] = math.Min(1, upperSum)
	}
	return out, nil
}

// Timeout makes observations exceeding deadline incorrect (+Inf mass).
func Timeout(child Band, deadline float64) (Band, error) {
	if err := child.Validate(); err != nil {
		return Band{}, err
	}
	if deadline < 0 || math.IsInf(deadline, 0) || math.IsNaN(deadline) {
		return Band{}, fmt.Errorf("invalid timeout")
	}
	set := map[float64]bool{deadline: true}
	for _, x := range child.Grid {
		if x <= deadline {
			set[x] = true
		}
	}
	grid := make([]float64, 0, len(set))
	for x := range set {
		grid = append(grid, x)
	}
	sort.Float64s(grid)
	out := Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid)), IntervalCensored: child.IntervalCensored}
	for i, x := range grid {
		out.Lower[i], out.Upper[i] = child.At(math.Min(x, deadline))
	}
	return out, nil
}

func unionGrid(bands ...Band) []float64 {
	set := map[float64]bool{}
	for _, band := range bands {
		for _, x := range band.Grid {
			set[x] = true
		}
	}
	grid := make([]float64, 0, len(set))
	for x := range set {
		grid = append(grid, x)
	}
	sort.Float64s(grid)
	return grid
}

func operatorGrid(leaves []Band) ([]float64, bool, error) {
	censored := false
	for _, leaf := range leaves {
		if err := leaf.Validate(); err != nil {
			return nil, false, err
		}
		censored = censored || leaf.IntervalCensored
	}
	return unionGrid(leaves...), censored, nil
}
