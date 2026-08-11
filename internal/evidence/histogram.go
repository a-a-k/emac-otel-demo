package evidence

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
)

type HistogramKey struct {
	Operation     string `json:"operation"`
	Branch        string `json:"branch"`
	EvidenceBlock string `json:"evidence_block"`
}

type Histogram struct {
	Bounds       []float64 `json:"bounds_ms"`
	BucketCounts []int     `json:"bucket_counts"`
}

// ReadHistograms reads every delta export and sums matching explicit
// histogram series. The final BucketCounts entry is the real +Inf overflow.
func ReadHistograms(path string) (map[HistogramKey]Histogram, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[HistogramKey]Histogram{}
	decoder := json.NewDecoder(f)
	for {
		var value any
		if err := decoder.Decode(&value); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if err := walkMetrics(value, "", out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func walkMetrics(value any, metricName string, out map[HistogramKey]Histogram) error {
	switch v := value.(type) {
	case []any:
		for _, child := range v {
			if err := walkMetrics(child, metricName, out); err != nil {
				return err
			}
		}
	case map[string]any:
		if name, ok := v["name"].(string); ok {
			metricName = name
		}
		if rawBounds, boundsOK := v["explicitBounds"].([]any); boundsOK {
			rawBuckets, bucketsOK := v["bucketCounts"].([]any)
			if bucketsOK && (metricName == "traces.span.metrics.duration" || metricName == "emac.policy.residual.duration") {
				attrs := attributes(v["attributes"])
				operation := attrs["emac.operation"]
				if metricName == "emac.policy.residual.duration" {
					operation = "policy.residual"
				}
				key := HistogramKey{Operation: operation, Branch: attrs["emac.branch"], EvidenceBlock: attrs["emac.evidence_block"]}
				if key.Operation == "" || key.Branch == "" || key.EvidenceBlock == "" {
					return fmt.Errorf("histogram is missing registered dimensions: %#v", key)
				}
				bounds := floats(rawBounds)
				buckets := ints(rawBuckets)
				if len(buckets) != len(bounds)+1 {
					return fmt.Errorf("histogram %v has %d bounds and %d buckets", key, len(bounds), len(buckets))
				}
				current, exists := out[key]
				if !exists {
					out[key] = Histogram{Bounds: bounds, BucketCounts: buckets}
					return nil
				}
				if !sameFloats(current.Bounds, bounds) || len(current.BucketCounts) != len(buckets) {
					return fmt.Errorf("histogram grid changed for %v", key)
				}
				for i := range buckets {
					current.BucketCounts[i] += buckets[i]
				}
				out[key] = current
				return nil
			}
		}
		for _, child := range v {
			if err := walkMetrics(child, metricName, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// HistogramPrefix sums registered evidence blocks ending at or before look.
func HistogramPrefix(all map[HistogramKey]Histogram, operation, branch string, look int) (Histogram, error) {
	var result Histogram
	for key, histogram := range all {
		if key.Operation != operation || key.Branch != branch {
			continue
		}
		end, err := strconv.Atoi(key.EvidenceBlock)
		if err != nil || end > look {
			continue
		}
		if result.Bounds == nil {
			result.Bounds = append([]float64(nil), histogram.Bounds...)
			result.BucketCounts = make([]int, len(histogram.BucketCounts))
		}
		if !sameFloats(result.Bounds, histogram.Bounds) {
			return Histogram{}, fmt.Errorf("histogram grid differs across evidence blocks for %s|%s", operation, branch)
		}
		for i, count := range histogram.BucketCounts {
			result.BucketCounts[i] += count
		}
	}
	if result.Bounds == nil {
		return Histogram{}, fmt.Errorf("no histogram for %s|%s through look %d", operation, branch, look)
	}
	return result, nil
}

func (h Histogram) Count() int {
	total := 0
	for _, n := range h.BucketCounts {
		total += n
	}
	return total
}

func floats(values []any) []float64 {
	out := make([]float64, len(values))
	for i, value := range values {
		switch v := value.(type) {
		case float64:
			out[i] = v
		case string:
			out[i], _ = strconv.ParseFloat(v, 64)
		case json.Number:
			out[i], _ = v.Float64()
		}
	}
	return out
}

func ints(values []any) []int {
	out := make([]int, len(values))
	for i, value := range values {
		out[i] = int(integer(value))
	}
	return out
}

func sameFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			return false
		}
	}
	return true
}

func HistogramKeys(histograms map[HistogramKey]Histogram) []HistogramKey {
	keys := make([]HistogramKey, 0, len(histograms))
	for key := range histograms {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Branch != keys[j].Branch {
			return keys[i].Branch < keys[j].Branch
		}
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		return keys[i].EvidenceBlock < keys[j].EvidenceBlock
	})
	return keys
}
