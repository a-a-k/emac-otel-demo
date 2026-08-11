package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/a-a-k/emac-otel-demo/internal/analysis"
	"github.com/a-a-k/emac-otel-demo/internal/evidence"
	"github.com/a-a-k/emac-otel-demo/internal/model"
	"github.com/a-a-k/emac-otel-demo/internal/statistics"
)

func TestReplayIdentifyingTrajectory(t *testing.T) {
	root := t.TempDir()
	labels := map[int]statistics.OracleLabel{10: statistics.Safe, 25: statistics.Safe, 50: statistics.Safe, 75: statistics.Unsafe, 100: statistics.Unsafe}
	green := true
	for _, weight := range []int{10, 25, 50, 75, 100} {
		for _, pipeline := range []string{"100", "25", "05"} {
			value := analysis.Result{
				Admission: evidence.Admission{Admitted: true}, IntegrityValid: true,
				Full:          model.CompileOutput{LowerAtDeadline: 1, UpperAtDeadline: 1},
				FeatureAware:  model.CompileOutput{LowerAtDeadline: 1, UpperAtDeadline: 1},
				CurrentOracle: analysis.Oracle{Label: statistics.Safe}, EvaluationOracle: analysis.Oracle{Label: labels[weight]},
				ComponentGreen: map[string]*bool{"leaf|stable_domestic": &green},
			}
			if weight == 50 {
				value.Full = model.CompileOutput{LowerAtDeadline: 0, UpperAtDeadline: .9}
				value.FeatureAware = value.Full
			}
			for _, look := range []int{1000, 2000, 4000} {
				path := filepath.Join(root, fmt.Sprintf("weight-%03d", weight), fmt.Sprintf("analysis-cap-04000-look-%05d-p%s.json", look, pipeline))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				b, _ := json.Marshal(value)
				if err := os.WriteFile(path, b, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	result, err := Replay(root, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Identifying {
		t.Fatalf("not identifying: %v", result.IdentifyReason)
	}
	full := result.Methods["FullEmaC"]
	if len(full.Decisions) != 3 || !full.Outcome.A || !full.Outcome.Z {
		t.Fatalf("full=%#v", full)
	}
}
