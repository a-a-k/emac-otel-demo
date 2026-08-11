package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func view(observation int64, count int) ProjectionView {
	v := ProjectionView{Observation: observation, Available: true, Snapshot: &Snapshot{}}
	v.Snapshot.Discovery.Edges = []Edge{{ID: "policy|cart", From: "checkout-policy", To: "cart", Identity: map[string]string{"operation": "GetCart"}, Support: Support{count}}}
	return v
}
func TestAdmissionUsesCumulativeWindowsAndStableCore(t *testing.T) {
	r := []RequiredEdge{{"checkout-policy", "cart", "GetCart"}}
	a := Admit([]ProjectionView{view(1, 4), view(2, 6)}, view(2, 6), r, 10, false)
	if !a.Admitted {
		t.Fatal(a.Reasons)
	}
	a = Admit([]ProjectionView{view(1, 9), view(3, 9)}, view(3, 9), r, 10, false)
	if a.Admitted {
		t.Fatal("version gap admitted")
	}
}

func TestAdmissionAcceptsEarlierConsecutiveRecurrence(t *testing.T) {
	r := []RequiredEdge{{"checkout-policy", "cart", "GetCart"}}
	windows := []ProjectionView{view(1, 5), view(2, 5), {Name: "raw_window", Observation: 3, Available: false}}
	a := Admit(windows, view(3, 1), r, 10, false)
	if !a.Admitted {
		t.Fatal(a.Reasons)
	}
}

func TestExtractProjectionSnapshotPreservesSchemaEnvelope(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "projection.json")
	output := filepath.Join(dir, "snapshot.json")
	raw := `{"available":true,"snapshot":{"metadata":{"schema":{"name":"io.mb3r.bering.snapshot","version":"1.3.0"}},"model":{"services":[{"id":"policy"}]}}}`
	if err := os.WriteFile(input, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ExtractProjectionSnapshot(input, output); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	b, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &snapshot); err != nil {
		t.Fatal(err)
	}
	metadata := snapshot["metadata"].(map[string]any)
	schema := metadata["schema"].(map[string]any)
	if schema["name"] != "io.mb3r.bering.snapshot" {
		t.Fatalf("schema metadata was not preserved: %#v", snapshot)
	}
}
