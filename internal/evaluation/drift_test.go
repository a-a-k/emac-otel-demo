package evaluation

import (
	"testing"

	"github.com/a-a-k/emac-otel-demo/internal/analysis"
	"github.com/a-a-k/emac-otel-demo/internal/evidence"
)

func TestHistogramDistanceUsesIntendedFailureMass(t *testing.T) {
	a := analysis.LeafEvidence{Intended: 10, Histogram: evidence.Histogram{Bounds: []float64{1, 2}, BucketCounts: []int{2, 3, 0}}}
	b := analysis.LeafEvidence{Intended: 20, Histogram: evidence.Histogram{Bounds: []float64{1, 2}, BucketCounts: []int{4, 6, 0}}}
	distance, err := histogramDistance(a, b)
	if err != nil || distance != 0 {
		t.Fatalf("distance=%g err=%v", distance, err)
	}
}

func TestPoissonBootstrapP95Ratio(t *testing.T) {
	h := evidence.Histogram{Bounds: []float64{1, 2, 3}, BucketCounts: []int{10, 80, 10, 0}}
	point, lower, upper := bootstrapP95Ratio(h, h, .05/72, 42)
	if point != 1 || lower <= 0 || upper < lower {
		t.Fatalf("point=%g interval=[%g,%g]", point, lower, upper)
	}
}
