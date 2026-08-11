package statistics

import (
	"math"
	"testing"
)

func TestMcNemarAndHolm(t *testing.T) {
	p, err := ExactMcNemarTwoSided(0, 10)
	if err != nil || math.Abs(p-.001953125) > 1e-12 {
		t.Fatalf("p=%g err=%v", p, err)
	}
	h, err := Holm(map[string]float64{"a": .001, "b": .02, "c": .2}, .05)
	if err != nil || !h[0].Reject || !h[1].Reject || h[2].Reject {
		t.Fatalf("%#v %v", h, err)
	}
}
