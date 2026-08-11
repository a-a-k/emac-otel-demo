package statistics

import (
	"math"
	"testing"
)

func TestClopperPearsonKnownValues(t *testing.T) {
	ci, err := ClopperPearson(5, 10, .05)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ci.Lower-.187086) < 1e-5 || math.Abs(ci.Upper-.812914) > 1e-5 {
		// The first comparison is intentionally split below for readable errors.
	}
	if math.Abs(ci.Lower-.187086) > 1e-5 || math.Abs(ci.Upper-.812914) > 1e-5 {
		t.Fatalf("got %#v", ci)
	}
	all, _ := ClopperPearson(10, 10, .05)
	if all.Upper != 1 || math.Abs(all.Lower-.691503) > 1e-5 {
		t.Fatalf("all-success %#v", all)
	}
}

func TestOracleLabels(t *testing.T) {
	counts := []CohortCount{{"c", 1000, 1000, .15}, {"si", 1000, 1000, .45}, {"sd", 1000, 1000, .4}}
	_, label, err := StratifiedOracle(counts, .05, .95)
	if err != nil || label != Safe {
		t.Fatalf("%s %v", label, err)
	}
}
