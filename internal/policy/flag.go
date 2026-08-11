package policy

import (
	"context"
	"fmt"
	"github.com/open-feature/go-sdk/openfeature"
)

type FlagAssigner struct{ Client *openfeature.Client }

func (f FlagAssigner) Candidate(ctx context.Context, international bool, bucket float64) (bool, error) {
	if f.Client == nil {
		return false, fmt.Errorf("nil flagd client")
	}
	details, err := f.Client.BooleanValueDetails(ctx, "candidateRouting", false, openfeature.NewTargetlessEvaluationContext(map[string]any{"international": international, "bucket": bucket}))
	if err != nil {
		return false, err
	}
	return details.Value, nil
}
