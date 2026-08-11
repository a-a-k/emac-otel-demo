package ledger

import (
	"errors"
	"fmt"
	"math"
	"time"
)

var ErrEvidenceConflict = errors.New("evidence conflict")

type Call struct {
	Operation string        `json:"operation"`
	Intended  bool          `json:"intended"`
	Attempted bool          `json:"attempted"`
	SpanCount int           `json:"span_count"`
	Correct   bool          `json:"correct"`
	Duration  time.Duration `json:"duration"`
}

type Request struct {
	RunID       string        `json:"run_id"`
	StageID     string        `json:"stage_id"`
	RequestID   string        `json:"request_id"`
	TraceID     string        `json:"trace_id"`
	Phase       string        `json:"phase"`
	Branch      string        `json:"branch"`
	RootCorrect bool          `json:"root_correct"`
	Root        time.Duration `json:"root_duration"`
	Calls       []Call        `json:"calls"`
}

type Observation struct {
	Operation string
	Attempted bool
	Correct   bool
	Duration  float64 // milliseconds; +Inf represents error/lawful-skip mass
}

func ValidateAndProject(r Request, negativeTolerance time.Duration) ([]Observation, time.Duration, error) {
	if r.Root < 0 {
		return nil, 0, fmt.Errorf("%w: negative root", ErrEvidenceConflict)
	}
	var executed time.Duration
	result := make([]Observation, 0, len(r.Calls)+1)
	for _, c := range r.Calls {
		if c.Attempted && c.SpanCount != 1 {
			return nil, 0, fmt.Errorf("%w: attempted %s has %d CLIENT spans", ErrEvidenceConflict, c.Operation, c.SpanCount)
		}
		if !c.Attempted && c.SpanCount != 0 {
			return nil, 0, fmt.Errorf("%w: unattempted %s has CLIENT span", ErrEvidenceConflict, c.Operation)
		}
		if c.Attempted && c.Duration < 0 {
			return nil, 0, fmt.Errorf("%w: negative duration for %s", ErrEvidenceConflict, c.Operation)
		}
		if c.Attempted {
			executed += c.Duration
		}
		if !c.Intended {
			continue
		}
		o := Observation{Operation: c.Operation, Attempted: c.Attempted, Correct: c.Attempted && c.Correct, Duration: math.Inf(1)}
		if o.Correct {
			o.Duration = float64(c.Duration) / float64(time.Millisecond)
		}
		result = append(result, o)
	}
	residual := r.Root - executed
	if residual < -negativeTolerance {
		return nil, 0, fmt.Errorf("%w: residual %s is below tolerance -%s", ErrEvidenceConflict, residual, negativeTolerance)
	}
	if residual < 0 {
		residual = 0
	}
	result = append(result, Observation{
		Operation: "policy.residual", Attempted: true, Correct: r.RootCorrect,
		Duration: func() float64 {
			if r.RootCorrect {
				return float64(residual) / float64(time.Millisecond)
			}
			return math.Inf(1)
		}(),
	})
	return result, residual, nil
}
