package statistics

import "testing"

func TestCounterfactualShare(t *testing.T) {
	v, err := CounterfactualShare(100, 100, .6, .05)
	if err != nil || v.Q.Upper != .6 || v.Q.Lower <= 0 {
		t.Fatalf("%#v %v", v, err)
	}
	d, err := TwoSampleDKW(.01, 1200, 3000, .05, true)
	if err != nil || d.Detected || !d.DecisionChanged {
		t.Fatalf("%#v %v", d, err)
	}
}
