package model

import "testing"

func TestCompileRequiresDeclaredLeaves(t *testing.T) {
	d := Band{Grid: []float64{1}, Lower: []float64{1}, Upper: []float64{1}}
	in := CompileInput{Leaves: map[string]Band{"c": d, "si": d, "sd": d}, Candidate: []string{"c"}, StableInternational: []string{"si"}, StableDomestic: []string{"sd"}, Eligibility: .6, HLower: .1, HUpper: .2, Deadline: 1}
	out, err := CompileThreeCohort(in)
	if err != nil || out.LowerAtDeadline != 1 {
		t.Fatalf("%#v %v", out, err)
	}
	in.Candidate = []string{"missing"}
	if _, err = CompileThreeCohort(in); err == nil {
		t.Fatal("missing binding accepted")
	}
}
