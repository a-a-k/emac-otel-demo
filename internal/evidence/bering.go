package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Support struct {
	TraceCount int `json:"trace_count"`
}
type Edge struct {
	ID, From, To string
	Identity     map[string]string `json:"identity"`
	Support      Support           `json:"support"`
}
type Snapshot struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	Ingest      struct {
		Traces       int `json:"traces"`
		DroppedSpans int `json:"dropped_spans"`
		LateSpans    int `json:"late_spans"`
	} `json:"ingest"`
	Discovery struct {
		Edges []Edge `json:"edges"`
	} `json:"discovery"`
}
type ProjectionView struct {
	Name        string    `json:"name"`
	Observation int64     `json:"observation"`
	Available   bool      `json:"available"`
	Snapshot    *Snapshot `json:"snapshot"`
}
type Report struct {
	Versions struct {
		Observation int64 `json:"observation_version"`
	} `json:"versions"`
}

type RequiredEdge struct{ From, To, Operation string }
type Admission struct {
	Admitted            bool           `json:"admitted"`
	Reasons             []string       `json:"reasons"`
	Cumulative          map[string]int `json:"cumulative_trace_count"`
	ObservationVersions []int64        `json:"observation_versions"`
}

func Admit(windows []ProjectionView, stable ProjectionView, required []RequiredEdge, floor int, evidenceConflict bool) Admission {
	a := Admission{Cumulative: map[string]int{}}
	if evidenceConflict {
		a.Reasons = append(a.Reasons, "ledger evidence conflict")
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].Observation < windows[j].Observation })
	for i, w := range windows {
		a.ObservationVersions = append(a.ObservationVersions, w.Observation)
		if i > 0 && w.Observation != windows[i-1].Observation+1 {
			a.Reasons = append(a.Reasons, "non-contiguous observation versions")
		}
		if !w.Available || w.Snapshot == nil {
			continue
		}
		if w.Snapshot.Ingest.DroppedSpans != 0 {
			a.Reasons = append(a.Reasons, fmt.Sprintf("observation %d dropped spans", w.Observation))
		}
		if w.Snapshot.Ingest.LateSpans != 0 {
			a.Reasons = append(a.Reasons, fmt.Sprintf("observation %d late spans", w.Observation))
		}
		for _, r := range required {
			if e, ok := findEdge(w.Snapshot.Edges(), r); ok {
				a.Cumulative[edgeKey(r)] += e.Support.TraceCount
			}
		}
		for _, edge := range w.Snapshot.Edges() {
			if edge.From == "checkout-policy" && !matchesAny(edge, required) {
				a.Reasons = append(a.Reasons, fmt.Sprintf("unregistered checkout-policy edge %s", edge.ID))
			}
		}
	}
	for _, r := range required {
		key := edgeKey(r)
		if a.Cumulative[key] < floor {
			a.Reasons = append(a.Reasons, fmt.Sprintf("%s cumulative trace_count %d < %d", key, a.Cumulative[key], floor))
		}
		if !hasConsecutiveEdge(windows, r) {
			a.Reasons = append(a.Reasons, key+" lacks two-window recurrence")
		}
		if !hasEdge(stable, r) {
			a.Reasons = append(a.Reasons, key+" absent from stable_core")
		}
	}
	a.Admitted = len(a.Reasons) == 0
	return a
}

func hasConsecutiveEdge(windows []ProjectionView, required RequiredEdge) bool {
	for i := 1; i < len(windows); i++ {
		if windows[i].Observation == windows[i-1].Observation+1 && hasEdge(windows[i-1], required) && hasEdge(windows[i], required) {
			return true
		}
	}
	return false
}

func matchesAny(edge Edge, required []RequiredEdge) bool {
	for _, candidate := range required {
		if _, ok := findEdge([]Edge{edge}, candidate); ok {
			return true
		}
	}
	return false
}

func (s *Snapshot) Edges() []Edge {
	if s == nil {
		return nil
	}
	return s.Discovery.Edges
}
func edgeKey(r RequiredEdge) string { return r.From + "->" + r.To + ":" + r.Operation }
func hasEdge(v ProjectionView, r RequiredEdge) bool {
	if !v.Available || v.Snapshot == nil {
		return false
	}
	_, ok := findEdge(v.Snapshot.Edges(), r)
	return ok
}
func findEdge(edges []Edge, r RequiredEdge) (Edge, bool) {
	for _, e := range edges {
		op := e.Identity["operation"]
		if e.From == r.From && e.To == r.To && (r.Operation == "" || op == r.Operation || strings.Contains(e.ID, "operation="+r.Operation)) {
			return e, true
		}
	}
	return Edge{}, false
}

func LoadProjection(path string) (ProjectionView, error) {
	var v ProjectionView
	b, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(b, &v)
	return v, err
}

// ExtractProjectionSnapshot preserves Bering's complete, schema-bound
// snapshot envelope. ProjectionView is deliberately reduced for admission
// analysis and must never be re-encoded for strict downstream consumers such
// as Sheaft.
func ExtractProjectionSnapshot(input, output string) error {
	b, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	var envelope struct {
		Available bool            `json:"available"`
		Snapshot  json.RawMessage `json:"snapshot"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return err
	}
	if !envelope.Available || len(envelope.Snapshot) == 0 || string(envelope.Snapshot) == "null" {
		return fmt.Errorf("Bering projection is unavailable")
	}
	var indented any
	if err := json.Unmarshal(envelope.Snapshot, &indented); err != nil {
		return err
	}
	out, err := json.MarshalIndent(indented, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(output, append(out, '\n'))
}
func LoadArchive(dir string) ([]ProjectionView, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "raw-windows", "observation-*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]ProjectionView, 0, len(paths))
	for _, p := range paths {
		v, err := LoadProjection(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func LoadStableArchive(dir string, observation int64) (ProjectionView, error) {
	return LoadProjection(filepath.Join(dir, "raw-windows", fmt.Sprintf("stable-%06d.json", observation)))
}

// Watch archives Bering's overwritten projection views once per observation.
// It copies only a mutually consistent raw/stable/report triple.
func Watch(dir, stopFile string, poll time.Duration) error {
	archive := filepath.Join(dir, "raw-windows")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return err
	}
	last := int64(0)
	for {
		if _, err := os.Stat(stopFile); err == nil {
			return nil
		}
		raw, err := LoadProjection(filepath.Join(dir, "latest-raw-window.json"))
		if err == nil && raw.Observation > last {
			stable, stableErr := LoadProjection(filepath.Join(dir, "latest-stable-core.json"))
			reportBytes, reportErr := os.ReadFile(filepath.Join(dir, "reconciliation-report.json"))
			var report Report
			if reportErr == nil {
				reportErr = json.Unmarshal(reportBytes, &report)
			}
			if stableErr == nil && reportErr == nil && stable.Observation == raw.Observation && report.Versions.Observation == raw.Observation {
				rawBytes, _ := json.MarshalIndent(raw, "", "  ")
				stableBytes, _ := json.MarshalIndent(stable, "", "  ")
				if err := atomicWrite(filepath.Join(archive, fmt.Sprintf("observation-%06d.json", raw.Observation)), rawBytes); err != nil {
					return err
				}
				if err := atomicWrite(filepath.Join(archive, fmt.Sprintf("stable-%06d.json", raw.Observation)), stableBytes); err != nil {
					return err
				}
				if err := atomicWrite(filepath.Join(archive, fmt.Sprintf("report-%06d.json", raw.Observation)), reportBytes); err != nil {
					return err
				}
				last = raw.Observation
			}
		}
		time.Sleep(poll)
	}
}
func atomicWrite(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
