package policy

import (
	"context"

	"github.com/a-a-k/emac-otel-demo/internal/experiment"
	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

type CartGRPC struct{ Client pb.CartServiceClient }

func (c CartGRPC) GetCart(ctx context.Context, userID string) (*pb.Cart, error) {
	ctx, span := startGRPC(ctx, "CartService/GetCart")
	defer span.End()
	v, err := c.Client.GetCart(ctx, &pb.GetCartRequest{UserId: userID})
	finishGRPC(span, err)
	return v, err
}

type CurrencyGRPC struct{ Client pb.CurrencyServiceClient }

func (c CurrencyGRPC) Supported(ctx context.Context) ([]string, error) {
	ctx, span := startGRPC(ctx, "CurrencyService/GetSupportedCurrencies")
	defer span.End()
	v, err := c.Client.GetSupportedCurrencies(ctx, &pb.Empty{})
	finishGRPC(span, err)
	if err != nil {
		return nil, err
	}
	return v.GetCurrencyCodes(), nil
}

func startGRPC(ctx context.Context, operation string) (context.Context, trace.Span) {
	m, _ := ctx.Value(metadataKey{}).(requestMetadata)
	if m.phase == experiment.PhaseWarmup {
		ctx = unsampledContext(ctx, m.requestID+":"+operation)
		carrier := propagation.MapCarrier{}
		otel.GetTextMapPropagator().Inject(ctx, carrier)
		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		for k, v := range carrier {
			md.Set(k, v)
		}
		return metadata.NewOutgoingContext(ctx, md), trace.SpanFromContext(ctx)
	}
	ctx, span := otel.Tracer("emac.checkout-policy").Start(ctx, operation, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(m.attributes(operation)...))
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	for k, v := range carrier {
		md.Set(k, v)
	}
	return metadata.NewOutgoingContext(ctx, md), span
}
func finishGRPC(span trace.Span, err error) {
	span.SetAttributes(attribute.Bool("emac.correct", err == nil))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
