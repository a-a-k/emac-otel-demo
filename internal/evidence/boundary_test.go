package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoundaryReconciliation(t *testing.T) {
	dir := t.TempDir()
	ledgerPath := filepath.Join(dir, "ledger.jsonl")
	logPath := filepath.Join(dir, "k6.log")
	if err := os.WriteFile(ledgerPath, []byte(`{"request_id":"r1","phase":"measured","root_correct":true,"root_duration":100000000}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(`EMAC_ORACLE {"request_id":"r1","phase":"measured","branch":"candidate","correct":true,"duration_ms":105}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ReconcileBoundary(ledgerPath, logPath, 200)
	if err != nil || !result.Valid || result.P95AbsoluteDeltaMS != 5 || result.LabelAgreement != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
