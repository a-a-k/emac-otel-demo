package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/a-a-k/emac-otel-demo/internal/controller"
)

func TestConfirmatoryFixedSixTestFamily(t *testing.T) {
	var paths []string
	for i := 0; i < 40; i++ {
		full := controller.TrajectoryOutcome{A: true, Z: true}
		baseline := controller.TrajectoryOutcome{A: i >= 35, Z: i >= 35}
		run := ReplayResult{Methods: map[string]MethodResult{
			"FullEmaC": {Outcome: full}, "Local": {Outcome: baseline}, "Reactive": {Outcome: baseline}, "FeatureAware": {Outcome: baseline},
		}}
		path := filepath.Join(t.TempDir(), fmt.Sprintf("run-%d.json", i))
		b, _ := json.Marshal(run)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	result, err := Confirmatory(paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.Runs != 40 || len(result.Tests) != 6 || len(result.Holm) != 6 {
		t.Fatal(result)
	}
}
