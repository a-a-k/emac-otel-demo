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
		data += `{"Name":"policy","CPUPerc":"` + string(rune('0'+i/10)) + string(rune('0'+i%10)) + `.0%","MemPerc":"10.0%"}` + "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cpu, memory, err := resourceP95(path)
	if err != nil || cpu != 19 || memory != 10 {
		t.Fatalf("cpu=%v memory=%v err=%v", cpu, memory, err)
	}
}
