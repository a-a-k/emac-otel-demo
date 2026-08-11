package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectHistogramCountsFiltersCorrectSpans(t *testing.T) {
	value := map[string]any{
		"name": "traces.span.metrics.duration",
		"histogram": map[string]any{"dataPoints": []any{
			dataPoint("CartService/GetCart", "candidate", true, "3"),
			dataPoint("CartService/GetCart", "candidate", false, "7"),
		}},
	}
	counts := map[string]int64{}
	collectHistogramCounts(value, counts)
	if counts["CartService/GetCart|candidate"] != 3 {
		t.Fatal(counts)
	}
}

func TestReconcileAllRegisteredHistograms(t *testing.T) {
	dir := t.TempDir()
	ledgerPath := filepath.Join(dir, "ledger.jsonl")
	metricsPath := filepath.Join(dir, "metrics.json")
	ledger := `{"phase":"measured","branch":"candidate","root_correct":true,"calls":[{"operation":"CartService/GetCart","attempted":true,"correct":true}]}` + "\n"
	if err := os.WriteFile(ledgerPath, []byte(ledger), 0o600); err != nil {
		t.Fatal(err)
	}
	metrics := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[` +
		metricJSON("traces.span.metrics.duration", "POST /api/checkout", "candidate", true) + `,` +
		metricJSON("traces.span.metrics.duration", "CartService/GetCart", "candidate", true) + `,` +
		metricJSON("emac.policy.residual.duration", "policy.residual", "candidate", true) +
		`]}]}]}`
	if err := os.WriteFile(metricsPath, []byte(metrics), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ReconcileResidualMetric(ledgerPath, metricsPath)
	if err != nil || !result.Valid {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func dataPoint(operation, branch string, correct bool, count string) map[string]any {
	return map[string]any{"attributes": []any{
		map[string]any{"key": "emac.operation", "value": map[string]any{"stringValue": operation}},
		map[string]any{"key": "emac.branch", "value": map[string]any{"stringValue": branch}},
		map[string]any{"key": "emac.correct", "value": map[string]any{"boolValue": correct}},
	}, "count": count}
}

func metricJSON(name, operation, branch string, correct bool) string {
	return `{"name":"` + name + `","histogram":{"dataPoints":[{"attributes":[` +
		`{"key":"emac.operation","value":{"stringValue":"` + operation + `"}},` +
		`{"key":"emac.branch","value":{"stringValue":"` + branch + `"}},` +
		`{"key":"emac.correct","value":{"boolValue":` + map[bool]string{true: "true", false: "false"}[correct] + `}}` +
		`],"count":"1"}]}}`
}
