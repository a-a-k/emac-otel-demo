package controller

import "testing"

func TestFullEmaC(t *testing.T) {
	if FullEmaC(false, 1, 1, .95) != Review {
		t.Fatal("inadmissible must review")
	}
	if FullEmaC(true, .96, .99, .95) != Pass {
		t.Fatal("expected pass")
	}
	if FullEmaC(true, .8, .94, .95) != Block {
		t.Fatal("expected block")
	}
}

func TestOracleModelAndEagerAblations(t *testing.T) {
	oracleModel := Machine{Method: OracleModel}
	decision, consumed := oracleModel.Step(StageInput{FeatureEvidence: true, FullLower: .80, FullUpper: .94, Target: .95, FinalLook: true})
	if !consumed || decision != Block {
		t.Fatalf("oracle model decision=%s consumed=%v", decision, consumed)
	}
	eager := Machine{Method: Eager}
	decision, consumed = eager.Step(StageInput{FinalLook: true})
	if !consumed || decision != Pass {
		t.Fatalf("eager decision=%s consumed=%v", decision, consumed)
	}
}
