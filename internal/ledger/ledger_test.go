package ledger

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestProjectionDistinguishesSkipAndConflict(t *testing.T) {
	r := Request{RootCorrect: false, Root: 15 * time.Millisecond, Calls: []Call{
		{Operation: "cart", Intended: true, Attempted: true, SpanCount: 1, Correct: false, Duration: 10 * time.Millisecond},
		{Operation: "currency", Intended: true, Attempted: false, SpanCount: 0},
	}}
	obs, residual, err := ValidateAndProject(r, time.Microsecond)
	if err != nil {
		t.Fatal(err)
	}
	if residual != 5*time.Millisecond {
		t.Fatalf("residual %s", residual)
	}
	if !math.IsInf(obs[0].Duration, 1) || !math.IsInf(obs[1].Duration, 1) {
		t.Fatal("error/skip not +Inf")
	}
	r.Calls[0].SpanCount = 0
	_, _, err = ValidateAndProject(r, time.Microsecond)
	if !errors.Is(err, ErrEvidenceConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestResidualTolerance(t *testing.T) {
	r := Request{RootCorrect: true, Root: 10 * time.Millisecond, Calls: []Call{{Operation: "x", Intended: true, Attempted: true, SpanCount: 1, Correct: true, Duration: 10*time.Millisecond + 500*time.Nanosecond}}}
	_, residual, err := ValidateAndProject(r, time.Microsecond)
	if err != nil || residual != 0 {
		t.Fatalf("got residual=%s err=%v", residual, err)
	}
}
