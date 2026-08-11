package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAndAggregateHistogramBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	document := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"traces.span.metrics.duration","histogram":{"dataPoints":[{"attributes":[{"key":"emac.operation","value":{"stringValue":"CartService/GetCart"}},{"key":"emac.branch","value":{"stringValue":"candidate"}},{"key":"emac.evidence_block","value":{"stringValue":"1000"}}],"count":"3","explicitBounds":[1,2],"bucketCounts":[1,1,1]}]}}]}]}]}
{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"traces.span.metrics.duration","histogram":{"dataPoints":[{"attributes":[{"key":"emac.operation","value":{"stringValue":"CartService/GetCart"}},{"key":"emac.branch","value":{"stringValue":"candidate"}},{"key":"emac.evidence_block","value":{"stringValue":"2000"}}],"count":"2","explicitBounds":[1,2],"bucketCounts":[0,2,0]}]}}]}]}]}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	all, err := ReadHistograms(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := HistogramPrefix(all, "CartService/GetCart", "candidate", 1000)
	if err != nil || first.Count() != 3 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := HistogramPrefix(all, "CartService/GetCart", "candidate", 2000)
	if err != nil || second.Count() != 5 || second.BucketCounts[1] != 3 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}
