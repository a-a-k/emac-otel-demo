package model

import (
	"fmt"
	"math"
	"sort"
)

type Band struct{ Grid, Lower, Upper []float64 }

func (b Band) Validate() error {
	if len(b.Grid) == 0 || len(b.Grid) != len(b.Lower) || len(b.Grid) != len(b.Upper) {
		return fmt.Errorf("band lengths differ or empty")
	}
	lastX, lastL, lastU := math.Inf(-1), 0.0, 0.0
	for i, x := range b.Grid {
		if x <= lastX {
			return fmt.Errorf("grid is not increasing")
		}
		l, u := b.Lower[i], b.Upper[i]
		if l < lastL || u < lastU || l < 0 || u > 1 || l > u {
			return fmt.Errorf("invalid CDF band at %d", i)
		}
		lastX, lastL, lastU = x, l, u
	}
	return nil
}

func (b Band) At(x float64) (float64, float64) {
	i := sort.Search(len(b.Grid), func(i int) bool { return b.Grid[i] > x }) - 1
	if i < 0 {
		return 0, 0
	}
	return b.Lower[i], b.Upper[i]
}

// Series2 computes bucket-grid recursive Makarov outer bounds without an
// independence assumption. The result grid contains all finite pair sums.
func Series2(a, b Band) (Band, error) {
	if err := a.Validate(); err != nil {
		return Band{}, err
	}
	if err := b.Validate(); err != nil {
		return Band{}, err
	}
	set := map[float64]struct{}{}
	for _, x := range a.Grid {
		for _, y := range b.Grid {
			set[x+y] = struct{}{}
		}
	}
	grid := make([]float64, 0, len(set))
	for x := range set {
		grid = append(grid, x)
	}
	sort.Float64s(grid)
	out := Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid))}
	for k, t := range grid {
		xs := append([]float64{}, a.Grid...)
		for _, y := range b.Grid {
			xs = append(xs, t-y)
		}
		lo, up := 0.0, 1.0
		for _, x := range xs {
			al, au := a.At(x)
			bl, bu := b.At(t - x)
			lo = math.Max(lo, math.Max(0, al+bl-1))
			up = math.Min(up, math.Min(1, au+bu))
		}
		if lo > up {
			lo = up
		}
		out.Lower[k], out.Upper[k] = lo, up
	}
	return out, nil
}

func Series(leaves ...Band) (Band, error) {
	if len(leaves) == 0 {
		return Band{}, fmt.Errorf("empty series")
	}
	all, err := allParenthesizations(leaves)
	if err != nil {
		return Band{}, err
	}
	return Intersect(all...)
}

func allParenthesizations(leaves []Band) ([]Band, error) {
	if len(leaves) == 1 {
		if err := leaves[0].Validate(); err != nil {
			return nil, err
		}
		return []Band{leaves[0]}, nil
	}
	var out []Band
	for split := 1; split < len(leaves); split++ {
		left, err := allParenthesizations(leaves[:split])
		if err != nil {
			return nil, err
		}
		right, err := allParenthesizations(leaves[split:])
		if err != nil {
			return nil, err
		}
		for _, a := range left {
			for _, b := range right {
				v, err := Series2(a, b)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
		}
	}
	return out, nil
}

// Intersect combines valid outer bounds. Across recursive Makarov
// parenthesizations this is max lower and min upper at each grid point.
func Intersect(bands ...Band) (Band, error) {
	if len(bands) == 0 {
		return Band{}, fmt.Errorf("empty intersection")
	}
	set := map[float64]struct{}{}
	for _, b := range bands {
		if err := b.Validate(); err != nil {
			return Band{}, err
		}
		for _, x := range b.Grid {
			set[x] = struct{}{}
		}
	}
	grid := make([]float64, 0, len(set))
	for x := range set {
		grid = append(grid, x)
	}
	sort.Float64s(grid)
	out := Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid))}
	for i, x := range grid {
		lo, up := 0.0, 1.0
		for _, b := range bands {
			l, u := b.At(x)
			lo = math.Max(lo, l)
			up = math.Min(up, u)
		}
		if lo > up {
			return Band{}, fmt.Errorf("empty bound intersection at %g", x)
		}
		out.Lower[i], out.Upper[i] = lo, up
	}
	return out, nil
}

// DKWBand constructs a finite-duration CDF band at frozen bucket boundaries.
// Error, lawful-skip, and finite-overflow observations remain in the intended
// denominator but outside the finite cumulative bucket counts.
func DKWBand(grid []float64, bucketCounts []int, intended int, alpha float64) (Band, error) {
	if len(grid) == 0 || len(grid) != len(bucketCounts) || intended <= 0 || alpha <= 0 || alpha >= 1 {
		return Band{}, fmt.Errorf("invalid DKW input")
	}
	eps := math.Sqrt(math.Log(2/alpha) / (2 * float64(intended)))
	out := Band{Grid: append([]float64(nil), grid...), Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid))}
	cumulative := 0
	last := -math.MaxFloat64
	for i, x := range grid {
		if x <= last || bucketCounts[i] < 0 {
			return Band{}, fmt.Errorf("invalid histogram")
		}
		last = x
		cumulative += bucketCounts[i]
		if cumulative > intended {
			return Band{}, fmt.Errorf("finite count exceeds intended")
		}
		f := float64(cumulative) / float64(intended)
		out.Lower[i] = math.Max(0, f-eps)
		out.Upper[i] = math.Min(1, f+eps)
	}
	return out, nil
}

// ThreeCohort mixes candidate, stable-international and stable-domestic CDF
// bands with fixed eligibility e and h in [hL,hU].
func ThreeCohort(candidate, stableInternational, stableDomestic Band, e, hL, hU float64) (Band, error) {
	for _, b := range []Band{candidate, stableInternational, stableDomestic} {
		if err := b.Validate(); err != nil {
			return Band{}, err
		}
	}
	if e < 0 || e > 1 || hL < 0 || hU > 1 || hL > hU {
		return Band{}, fmt.Errorf("invalid mixture weights")
	}
	set := map[float64]struct{}{}
	for _, b := range []Band{candidate, stableInternational, stableDomestic} {
		for _, x := range b.Grid {
			set[x] = struct{}{}
		}
	}
	grid := make([]float64, 0, len(set))
	for x := range set {
		grid = append(grid, x)
	}
	sort.Float64s(grid)
	out := Band{Grid: grid, Lower: make([]float64, len(grid)), Upper: make([]float64, len(grid))}
	for i, x := range grid {
		cl, cu := candidate.At(x)
		sil, siu := stableInternational.At(x)
		sdl, sdu := stableDomestic.At(x)
		lowerAt := func(h float64) float64 { return e*h*cl + e*(1-h)*sil + (1-e)*sdl }
		upperAt := func(h float64) float64 { return e*h*cu + e*(1-h)*siu + (1-e)*sdu }
		out.Lower[i] = math.Min(lowerAt(hL), lowerAt(hU))
		out.Upper[i] = math.Max(upperAt(hL), upperAt(hU))
	}
	return out, nil
}
