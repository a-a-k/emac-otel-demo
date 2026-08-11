package controller

type Decision string

const (
	Pass   Decision = "PASS"
	Block  Decision = "BLOCK"
	Review Decision = "REVIEW"
)

func FullEmaC(admitted bool, lowerAtDeadline, upperAtDeadline, target float64) Decision {
	if !admitted {
		return Review
	}
	if lowerAtDeadline >= target {
		return Pass
	}
	if upperAtDeadline < target {
		return Block
	}
	return Review
}

type TrajectoryOutcome struct {
	A bool `json:"A"`
	Z bool `json:"Z"`
}

type Method string

const (
	Local        Method = "Local"
	Reactive     Method = "Reactive"
	FeatureAware Method = "FeatureAware"
	Full         Method = "FullEmaC"
	Eager        Method = "Eager"
	Oracle       Method = "Oracle"
)

type StageInput struct {
	Admitted, FeatureEvidence                                bool
	FinalLook                                                bool
	ComponentGreen                                           map[string]*bool
	CurrentOracle                                            string
	FullLower, FullUpper, FeatureLower, FeatureUpper, Target float64
}
type Machine struct {
	Method   Method
	Terminal bool
	History  []Decision
}

// Step implements the registered deterministic virtual controllers. BLOCK and
// REVIEW are terminal, so a method cannot consume post-stop evidence.
func (m *Machine) Step(in StageInput) (Decision, bool) {
	if m.Terminal {
		return "", false
	}
	target := in.Target
	if target == 0 {
		target = .95
	}
	d := Review
	switch m.Method {
	case Local:
		d = Pass
		if len(in.ComponentGreen) == 0 {
			d = Review
		}
		for _, green := range in.ComponentGreen {
			if green == nil {
				d = Review
				break
			}
			if !*green {
				d = Block
				break
			}
		}
	case Reactive:
		d = oracleDecision(in.CurrentOracle)
	case FeatureAware:
		d = FullEmaC(in.FeatureEvidence, in.FeatureLower, in.FeatureUpper, target)
	case Full:
		d = FullEmaC(in.Admitted, in.FullLower, in.FullUpper, target)
	case Eager:
		d = Pass
	case Oracle:
		d = oracleDecision(in.CurrentOracle)
	default:
		d = Review
	}
	m.History = append(m.History, d)
	if d == Block || (d == Review && in.FinalLook) {
		m.Terminal = true
	}
	return d, true
}
func oracleDecision(label string) Decision {
	switch label {
	case "SAFE":
		return Pass
	case "UNSAFE":
		return Block
	default:
		return Review
	}
}

// Outcomes implements the registered A/Z rules. A stopped virtual trajectory
// can be shorter than the full-sweep oracle labels.
func Outcomes(labels []string, decisions []Decision) TrajectoryOutcome {
	firstUnsafe := -1
	a := true
	for i, l := range labels {
		if l == "UNSAFE" && i < len(decisions) && decisions[i] == Pass {
			a = false
		}
		if firstUnsafe < 0 && l == "UNSAFE" {
			firstUnsafe = i
		}
	}
	if firstUnsafe < 0 {
		return TrajectoryOutcome{A: a, Z: false}
	}
	z := firstUnsafe < len(decisions) && (decisions[firstUnsafe] == Block || decisions[firstUnsafe] == Review)
	for i := 0; i < firstUnsafe; i++ {
		if labels[i] != "SAFE" || i >= len(decisions) || decisions[i] != Pass {
			z = false
			break
		}
	}
	return TrajectoryOutcome{A: a, Z: z}
}
