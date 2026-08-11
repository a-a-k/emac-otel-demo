package evidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/a-a-k/emac-otel-demo/internal/ledger"
)

type MetricReconciliation struct {
	Valid     bool             `json:"valid"`
	Expected  map[string]int64 `json:"expected"`
	Observed  map[string]int64 `json:"observed"`
	Conflicts []string         `json:"conflicts,omitempty"`
}

func ReconcileResidualMetric(ledgerPath, metricsPath string) (MetricReconciliation, error) {
	out := MetricReconciliation{Expected: map[string]int64{}, Observed: map[string]int64{}}
	f, err := os.Open(ledgerPath)
	if err != nil {
		return out, err
	}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scan.Scan() {
		var r ledger.Request
		if err := json.Unmarshal(scan.Bytes(), &r); err != nil {
			f.Close()
			return out, err
		}
		if r.Phase != "measured" {
			continue
		}
		if r.RootCorrect {
			out.Expected[metricKey("POST /api/checkout", r.Branch)]++
			out.Expected[metricKey("policy.residual", r.Branch)]++
		}
		for _, call := range r.Calls {
			if call.Attempted && call.Correct {
				out.Expected[metricKey(call.Operation, r.Branch)]++
			}
		}
	}
	if err := scan.Err(); err != nil {
		f.Close()
		return out, err
	}
	if err := f.Close(); err != nil {
		return out, err
	}

	mf, err := os.Open(metricsPath)
	if err != nil {
		return out, err
	}
	defer mf.Close()
	decoder := json.NewDecoder(mf)
	for {
		var value any
		if err := decoder.Decode(&value); err != nil {
			if err == io.EOF {
				break
			}
			return out, err
		}
		collectHistogramCounts(value, out.Observed)
	}
	for key, want := range out.Expected {
		if got := out.Observed[key]; got != want {
			out.Conflicts = append(out.Conflicts, fmt.Sprintf("%s histogram count got %d want %d", key, got, want))
		}
	}
	for key, got := range out.Observed {
		if _, registered := out.Expected[key]; !registered && got != 0 {
			out.Conflicts = append(out.Conflicts, fmt.Sprintf("unexpected registered histogram %s count %d", key, got))
		}
	}
	out.Valid = len(out.Conflicts) == 0
	return out, nil
}

func collectHistogramCounts(value any, counts map[string]int64) {
	object, ok := value.(map[string]any)
	if !ok {
		if list, listOK := value.([]any); listOK {
			for _, child := range list {
				collectHistogramCounts(child, counts)
			}
		}
		return
	}
	name, _ := object["name"].(string)
	if name == "traces.span.metrics.duration" || name == "emac.policy.residual.duration" {
		collectDataPoints(object, name, counts)
		return
	}
	for _, child := range object {
		collectHistogramCounts(child, counts)
	}
}

func collectDataPoints(value any, metricName string, counts map[string]int64) {
	switch v := value.(type) {
	case map[string]any:
		if rawCount, exists := v["count"]; exists {
			attrs := attributes(v["attributes"])
			operation := attrs["emac.operation"]
			branch := attrs["emac.branch"]
			correct := attrs["emac.correct"]
			if metricName == "emac.policy.residual.duration" {
				operation = "policy.residual"
			}
			if operation != "" && branch != "" && (metricName != "traces.span.metrics.duration" || correct == "true") {
				counts[metricKey(operation, branch)] += integer(rawCount)
			}
			return
		}
		for _, child := range v {
			collectDataPoints(child, metricName, counts)
		}
	case []any:
		for _, child := range v {
			collectDataPoints(child, metricName, counts)
		}
	}
}

func attributes(value any) map[string]string {
	out := map[string]string{}
	list, _ := value.([]any)
	for _, raw := range list {
		item, _ := raw.(map[string]any)
		key, _ := item["key"].(string)
		wrapped, _ := item["value"].(map[string]any)
		for _, candidate := range []string{"stringValue", "boolValue", "intValue", "doubleValue"} {
			if v, exists := wrapped[candidate]; exists {
				out[key] = fmt.Sprint(v)
				break
			}
		}
	}
	return out
}

func integer(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

func metricKey(operation, branch string) string {
	return strings.TrimSpace(operation) + "|" + strings.TrimSpace(branch)
}
