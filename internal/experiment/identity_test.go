package experiment

import (
	"fmt"
	"testing"
)

func TestExactPersonaQuotaAndStableBucket(t *testing.T) {
	seeds := DeriveSeeds([]byte("registered-run-seed"))
	for block := uint64(0); block < 20; block++ {
		count := 0
		for i := uint64(0); i < 10; i++ {
			if International(seeds.Eligibility, "r1", block*10+i) {
				count++
			}
		}
		if count != 6 {
			t.Fatalf("block %d: got %d international", block, count)
		}
	}
	rk1, user1, _ := Identity(seeds, "r1", "10", 7)
	rk2, user2, _ := Identity(seeds, "r1", "25", 7)
	if rk1 != rk2 {
		t.Fatal("rollout key changed between stages")
	}
	if user1 == user2 {
		t.Fatal("user identity did not change between stages")
	}
	if Bucket(seeds.Rollout, rk1) != Bucket(seeds.Rollout, rk2) {
		t.Fatal("bucket changed")
	}
}

func TestNestedSampling(t *testing.T) {
	for i := 0; i < 10000; i++ {
		id := fmt.Sprintf("%032x", i+1)
		if Sample(id, .05) && !Sample(id, .25) {
			t.Fatal("5% not nested in 25%")
		}
		if Sample(id, .25) && !Sample(id, 1) {
			t.Fatal("25% not nested in 100%")
		}
	}
}

func TestAllEligibleStage(t *testing.T) {
	plan, err := BuildStagePlanWithPersona([]byte("seed"), "run", "100", 1, 0, 20, PhaseMeasured, "all-eligible")
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range plan.Requests {
		if !request.International || request.Branch != Candidate {
			t.Fatal(request)
		}
	}
}
