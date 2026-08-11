package evidence

import "testing"

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
