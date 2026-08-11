package experiment

import "testing"

func TestEvidenceBlock(t *testing.T) {
	for index, want := range map[int]string{-1: "excluded", 0: "1000", 999: "1000", 1000: "2000", 3999: "4000", 19999: "20000", 20000: "overflow"} {
		if got := EvidenceBlock(index); got != want {
			t.Errorf("EvidenceBlock(%d)=%s want %s", index, got, want)
		}
	}
}
