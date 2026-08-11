package experiment

import "strconv"

var evidenceLookEnds = []int{1000, 2000, 4000, 8000, 12000, 16000, 20000}

// EvidenceBlock returns the registered prefix boundary containing index.
// Histograms remain low-cardinality while exact prefix looks can be rebuilt
// by summing complete blocks.
func EvidenceBlock(index int) string {
	if index < 0 {
		return "excluded"
	}
	for _, end := range evidenceLookEnds {
		if index < end {
			return strconv.Itoa(end)
		}
	}
	return "overflow"
}
