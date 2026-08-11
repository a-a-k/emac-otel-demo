package policy

import (
	"context"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/ledger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	Intended, Correct metric.Int64Counter
	Residual          metric.Float64Histogram
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	intended, err := meter.Int64Counter("emac.policy.intended")
	if err != nil {
		return nil, err
	}
	correct, err := meter.Int64Counter("emac.policy.correct")
	if err != nil {
		return nil, err
	}
	residual, err := meter.Float64Histogram("emac.policy.residual.duration", metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}
	return &Metrics{intended, correct, residual}, nil
}
func (m *Metrics) Record(ctx context.Context, r ledger.Request) error {
	if r.Phase != "measured" {
		return nil
	}
	_, residual, err := ledger.ValidateAndProject(r, time.Microsecond)
	if err != nil {
		return err
	}
	attrs := metric.WithAttributes(attribute.String("emac.operation", "policy.residual"), attribute.String("emac.branch", r.Branch), attribute.String("emac.phase", r.Phase), attribute.String("emac.run_id", r.RunID), attribute.String("emac.stage_id", r.StageID))
	m.Intended.Add(ctx, 1, attrs)
	if r.RootCorrect {
		m.Correct.Add(ctx, 1, attrs)
		m.Residual.Record(ctx, float64(residual)/float64(time.Millisecond), attrs)
	}
	return nil
}
