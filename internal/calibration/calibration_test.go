package calibration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRegisteredCalibrationRule(t *testing.T) {
	dir := t.TempDir()
	stable, candidate := make([]string, 3), make([]string, 3)
	for i := 0; i < 3; i++ {
		stable[i] = filepath.Join(dir, fmt.Sprintf("stable-%d.jsonl", i))
		candidate[i] = filepath.Join(dir, fmt.Sprintf("candidate-%d.jsonl", i))
		stableLine := fmt.Sprintf(`{"phase":"measured","branch":"stable_domestic","root_correct":true,"root_duration":%d,"calls":[{"operation":"Frontend/POST api/checkout","intended":true,"attempted":true,"span_count":1,"correct":true,"duration":50000000}]}`+"\n"+`{"phase":"measured","branch":"stable_international","root_correct":true,"root_duration":100000000,"calls":[{"operation":"Frontend/POST api/checkout","intended":true,"attempted":true,"span_count":1,"correct":true,"duration":50000000}]}`+"\n", (100+i*10)*1_000_000)
		candidateLine := `{"phase":"measured","branch":"candidate","root_correct":true,"root_duration":200000000,"calls":[` +
			`{"operation":"CartService/GetCart","intended":true,"attempted":true,"span_count":1,"correct":true,"duration":30000000},` +
			`{"operation":"CurrencyService/GetSupportedCurrencies","intended":true,"attempted":true,"span_count":1,"correct":true,"duration":30000000},` +
			`{"operation":"Shipping/POST get-quote","intended":true,"attempted":true,"span_count":1,"correct":true,"duration":30000000},` +
			`{"operation":"Frontend/POST api/checkout","intended":true,"attempted":true,"span_count":1,"correct":true,"duration":30000000}]}` + "\n"
		if err := os.WriteFile(stable[i], []byte(stableLine), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(candidate[i], []byte(candidateLine), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Build(stable, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if result.JourneyDeadlineMS != 140 || len(result.HistogramGridMS) != 48 || result.HistogramGridMS[47] != 280 {
		t.Fatal(result)
	}
	if result.LocalDeadlinesMS["Frontend/POST api/checkout|stable_domestic"] != 60 {
		t.Fatal(result.LocalDeadlinesMS)
	}
	if baseline := result.Baselines["Frontend/POST api/checkout|stable_domestic"]; baseline.SuccessRate != 1 || baseline.P95MS != 50 {
		t.Fatal(baseline)
	}
}
