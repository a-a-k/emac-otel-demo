package controller

import "testing"

func TestMachineStopsWithoutLeakage(t *testing.T) {
	m := Machine{Method: Full}
	d, ok := m.Step(StageInput{Admitted: false})
	if !ok || d != Review {
		t.Fatal(d)
	}
	if _, ok = m.Step(StageInput{Admitted: true, FullLower: 1, FullUpper: 1}); ok {
		t.Fatal("terminal machine consumed later evidence")
	}
}
func TestLocalUnknownAndRed(t *testing.T) {
	yes, no := true, false
	m := Machine{Method: Local}
	d, _ := m.Step(StageInput{ComponentGreen: map[string]*bool{"a": &yes, "b": nil}})
	if d != Review {
		t.Fatal(d)
	}
	m = Machine{Method: Local}
	d, _ = m.Step(StageInput{ComponentGreen: map[string]*bool{"a": &yes, "b": &no}})
	if d != Block {
		t.Fatal(d)
	}
}

func TestRegisteredTrajectoryOutcomes(t *testing.T) {
	tests := []struct {
		labels    []string
		decisions []Decision
		want      TrajectoryOutcome
	}{
		{[]string{"SAFE", "SAFE"}, []Decision{Pass, Pass}, TrajectoryOutcome{A: true, Z: false}},
		{[]string{"SAFE", "UNSAFE"}, []Decision{Pass, Block}, TrajectoryOutcome{A: true, Z: true}},
		{[]string{"SAFE", "UNSAFE"}, []Decision{Pass, Review}, TrajectoryOutcome{A: true, Z: true}},
		{[]string{"SAFE", "UNSAFE"}, []Decision{Pass, Pass}, TrajectoryOutcome{A: false, Z: false}},
		{[]string{"SAFE", "UNSAFE"}, []Decision{Review}, TrajectoryOutcome{A: true, Z: false}},
		{[]string{"INDETERMINATE", "UNSAFE"}, []Decision{Pass, Block}, TrajectoryOutcome{A: true, Z: false}},
	}
	for _, test := range tests {
		if got := Outcomes(test.labels, test.decisions); got != test.want {
			t.Errorf("Outcomes(%v, %v) = %#v, want %#v", test.labels, test.decisions, got, test.want)
		}
	}
}
