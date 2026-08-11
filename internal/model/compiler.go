package model

import "fmt"

type CompileInput struct {
	Leaves              map[string]Band `json:"leaves"`
	Candidate           []string        `json:"candidate"`
	StableInternational []string        `json:"stable_international"`
	StableDomestic      []string        `json:"stable_domestic"`
	Eligibility         float64         `json:"eligibility"`
	HLower              float64         `json:"h_lower"`
	HUpper              float64         `json:"h_upper"`
	Deadline            float64         `json:"deadline"`
}
type CompileOutput struct {
	Journey         Band    `json:"journey"`
	Deadline        float64 `json:"deadline"`
	LowerAtDeadline float64 `json:"lower_at_deadline"`
	UpperAtDeadline float64 `json:"upper_at_deadline"`
}

func CompileThreeCohort(in CompileInput) (CompileOutput, error) {
	series := func(names []string) (Band, error) {
		if len(names) == 0 {
			return Band{}, fmt.Errorf("empty declared branch")
		}
		leaves := make([]Band, 0, len(names))
		for _, name := range names {
			b, ok := in.Leaves[name]
			if !ok {
				return Band{}, fmt.Errorf("missing declared leaf %q", name)
			}
			leaves = append(leaves, b)
		}
		return Series(leaves...)
	}
	c, err := series(in.Candidate)
	if err != nil {
		return CompileOutput{}, err
	}
	si, err := series(in.StableInternational)
	if err != nil {
		return CompileOutput{}, err
	}
	sd, err := series(in.StableDomestic)
	if err != nil {
		return CompileOutput{}, err
	}
	journey, err := ThreeCohort(c, si, sd, in.Eligibility, in.HLower, in.HUpper)
	if err != nil {
		return CompileOutput{}, err
	}
	l, u := journey.At(in.Deadline)
	return CompileOutput{journey, in.Deadline, l, u}, nil
}
