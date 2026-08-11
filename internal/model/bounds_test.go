package model

import "testing"

func TestSeriesAndThreeCohort(t *testing.T) {
	deterministic := func(x float64) Band {
		return Band{Grid: []float64{x}, Lower: []float64{1}, Upper: []float64{1}}
	}
	s, err := Series(deterministic(2), deterministic(3))
	if err != nil {
		t.Fatal(err)
	}
	l, u := s.At(4.99)
	if l != 0 || u != 0 {
		t.Fatalf("sum before support=%v,%v", l, u)
	}
	l, u = s.At(5)
	if l != 1 || u != 1 {
		t.Fatalf("sum at support=%v,%v", l, u)
	}
	mix, err := ThreeCohort(deterministic(10), deterministic(2), deterministic(1), .6, .25, .5)
	if err != nil {
		t.Fatal(err)
	}
	l, u = mix.At(2)
	if l < .69 || l > .71 || u < .84 || u > .86 {
		t.Fatalf("mixture=%v,%v", l, u)
	}
}

func TestDKWUsesIntendedDenominator(t *testing.T) {
	b, err := DKWBand([]float64{1, 2}, []int{400, 300}, 1000, .05)
	if err != nil {
		t.Fatal(err)
	}
	_, u := b.At(2)
	if u >= 1 {
		t.Fatalf("errors were lost from denominator: upper=%g", u)
	}
	if _, err = DKWBand([]float64{1}, []int{11}, 10, .05); err == nil {
		t.Fatal("accepted impossible histogram")
	}
}

func TestIntervalCensoringUsesNextBoundaryForUpper(t *testing.T) {
	b, err := DKWBand([]float64{1, 2}, []int{2, 8}, 10, .5)
	if err != nil {
		t.Fatal(err)
	}
	lower, upper := b.At(1.5)
	if lower > b.Lower[0] || upper < b.Upper[1] {
		t.Fatalf("inside bucket got [%g,%g], boundary bands %#v", lower, upper, b)
	}
}
