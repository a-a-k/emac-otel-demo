package evaluation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResourceP95(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.jsonl")
	data := ""
	for i := 1; i <= 20; i++ {
		data += `{"Name":"checkout-policy","CPUPerc":"` + string(rune('0'+i/10)) + string(rune('0'+i%10)) + `.0%","MemPerc":"10.0%"}` + "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cpu, memory, err := resourceP95(path)
	if err != nil || cpu != 19 || memory != 10 {
		t.Fatalf("cpu=%v memory=%v err=%v", cpu, memory, err)
	}
}

func TestReadK6SummaryV057(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.json")
	raw := `{"metrics":{"iterations":{"count":1201,"rate":5.004},"dropped_iterations":{"count":2,"rate":0.008}}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	count, dropped, rate, err := readK6Summary(path)
	if err != nil || count != 1201 || dropped != 2 || rate != 5.004 {
		t.Fatalf("count=%v dropped=%v rate=%v err=%v", count, dropped, rate, err)
	}
}

func TestCapacityScopeExcludesUnrelatedDemoServices(t *testing.T) {
	if capacityContainer("ad") || !capacityContainer("opentelemetry-demo-bering100-1") || !capacityContainer("product-catalog") {
		t.Fatal("unexpected capacity scope")
	}
}
