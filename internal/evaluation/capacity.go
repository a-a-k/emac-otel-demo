package evaluation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/a-a-k/emac-otel-demo/internal/evidence"
)

type CapacityResult struct {
	Valid                    bool     `json:"valid"`
	TargetRate               float64  `json:"target_rate"`
	AchievedRate             float64  `json:"achieved_rate"`
	IngressDeviationPercent  float64  `json:"ingress_deviation_percent"`
	DroppedIterationsPercent float64  `json:"dropped_iterations_percent"`
	CPUP95Percent            float64  `json:"cpu_p95_percent"`
	MemoryP95Percent         float64  `json:"memory_p95_percent"`
	TelemetryDrops           int      `json:"telemetry_drops"`
	LateSpans                int      `json:"late_spans"`
	Reasons                  []string `json:"reasons,omitempty"`
}

func Capacity(k6Summary, resources, beringDir string, targetRate float64) (CapacityResult, error) {
	return CapacityMany([]string{k6Summary}, resources, beringDir, targetRate)
}

// CapacityMany aggregates independently exported k6 segments into one stage
// result. Segmenting a stage at evidence looks must not change the registered
// ingress or dropped-iteration manipulation check.
func CapacityMany(k6Summaries []string, resources, beringDir string, targetRate float64) (CapacityResult, error) {
	result := CapacityResult{TargetRate: targetRate}
	if len(k6Summaries) == 0 {
		return result, fmt.Errorf("at least one k6 summary is required")
	}
	iterations, dropped, durationSeconds := 0.0, 0.0, 0.0
	for _, path := range k6Summaries {
		count, missed, rate, err := readK6Summary(path)
		if err != nil {
			return result, err
		}
		iterations += count
		dropped += missed
		if count > 0 && rate > 0 {
			durationSeconds += count / rate
		}
	}
	if durationSeconds > 0 {
		result.AchievedRate = iterations / durationSeconds
	}
	if iterations+dropped > 0 {
		result.DroppedIterationsPercent = 100 * dropped / (iterations + dropped)
	}
	if targetRate > 0 {
		result.IngressDeviationPercent = 100 * math.Abs(result.AchievedRate-targetRate) / targetRate
	}
	var err error
	result.CPUP95Percent, result.MemoryP95Percent, err = resourceP95(resources)
	if err != nil {
		return result, err
	}
	windows, err := evidence.LoadArchive(beringDir)
	if err != nil {
		return result, err
	}
	for _, window := range windows {
		if window.Available && window.Snapshot != nil {
			result.TelemetryDrops += window.Snapshot.Ingest.DroppedSpans
			result.LateSpans += window.Snapshot.Ingest.LateSpans
		}
	}
	if result.IngressDeviationPercent > 2 {
		result.Reasons = append(result.Reasons, fmt.Sprintf("ingress deviation %.3f%% > 2%%", result.IngressDeviationPercent))
	}
	if result.DroppedIterationsPercent >= 1 {
		result.Reasons = append(result.Reasons, fmt.Sprintf("dropped iterations %.3f%% >= 1%%", result.DroppedIterationsPercent))
	}
	if result.CPUP95Percent >= 70 {
		result.Reasons = append(result.Reasons, fmt.Sprintf("CPU p95 %.3f%% >= 70%%", result.CPUP95Percent))
	}
	if result.MemoryP95Percent >= 80 {
		result.Reasons = append(result.Reasons, fmt.Sprintf("memory p95 %.3f%% >= 80%%", result.MemoryP95Percent))
	}
	if result.TelemetryDrops != 0 || result.LateSpans != 0 {
		result.Reasons = append(result.Reasons, "telemetry drops or late spans are nonzero")
	}
	result.Valid = len(result.Reasons) == 0
	return result, nil
}

func readK6Summary(path string) (float64, float64, float64, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, 0, err
	}
	var summary struct {
		Metrics map[string]struct {
			Values map[string]float64 `json:"values"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(bytes, &summary); err != nil {
		return 0, 0, 0, err
	}
	iterations := summary.Metrics["iterations"].Values["count"]
	dropped := summary.Metrics["dropped_iterations"].Values["count"]
	rate := summary.Metrics["iterations"].Values["rate"]
	if iterations < 0 || dropped < 0 || rate < 0 {
		return 0, 0, 0, fmt.Errorf("invalid k6 metrics in %s", path)
	}
	return iterations, dropped, rate, nil
}

func resourceP95(path string) (float64, float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	byContainerCPU := map[string][]float64{}
	byContainerMemory := map[string][]float64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var sample map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			return 0, 0, err
		}
		name := fmt.Sprint(sample["Name"])
		byContainerCPU[name] = append(byContainerCPU[name], percentage(sample["CPUPerc"]))
		byContainerMemory[name] = append(byContainerMemory[name], percentage(sample["MemPerc"]))
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	maxCPU, maxMemory := 0.0, 0.0
	for name, values := range byContainerCPU {
		if name == "" {
			continue
		}
		maxCPU = math.Max(maxCPU, percentile(values, .95))
		maxMemory = math.Max(maxMemory, percentile(byContainerMemory[name], .95))
	}
	return maxCPU, maxMemory, nil
}

func percentage(value any) float64 {
	text := strings.TrimSuffix(strings.TrimSpace(fmt.Sprint(value)), "%")
	number, _ := strconv.ParseFloat(text, 64)
	return number
}

func percentile(values []float64, probability float64) float64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	index := int(math.Ceil(probability*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}
