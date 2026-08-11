package validation

import "testing"

func TestRQ1SoundnessSmoke(t *testing.T) {
	result, err := ValidateRQ1(20270811, 250)
	if err != nil || result.Violations != 0 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
