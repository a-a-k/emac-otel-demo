package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/experiment"
	"github.com/a-a-k/emac-otel-demo/internal/policy"
	flagd "github.com/open-feature/go-sdk-contrib/providers/flagd/pkg"
	"github.com/open-feature/go-sdk/openfeature"
	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil || response.StatusCode != http.StatusOK {
			if response != nil {
				_ = response.Body.Close()
			}
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx := context.Background()
	shutdown, meter, err := observability(ctx)
	if err != nil {
		return err
	}
	defer shutdown(context.Background())
	cartConn, err := grpc.NewClient(required("CART_ADDR"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer cartConn.Close()
	currencyConn, err := grpc.NewClient(required("CURRENCY_ADDR"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer currencyConn.Close()
	weight, err := strconv.ParseFloat(required("EMAC_WEIGHT"), 64)
	if err != nil {
		return err
	}
	writer, err := policy.OpenLedger(env("EMAC_LEDGER_PATH", "/var/lib/emac/ledger.jsonl"))
	if err != nil {
		return err
	}
	defer writer.Close()
	svc := &policy.Service{Config: policy.Config{FrontendURL: required("FRONTEND_ADDR"), ShippingURL: required("SHIPPING_ADDR"), Weight: weight, Seeds: experiment.DeriveSeeds([]byte(required("EMAC_RUN_SEED"))), Timeout: 30 * time.Second}, Cart: policy.CartGRPC{Client: pb.NewCartServiceClient(cartConn)}, Currency: policy.CurrencyGRPC{Client: pb.NewCurrencyServiceClient(currencyConn)}, Client: policy.NewHTTPClient(), Ledger: writer.Write}
	metrics, err := policy.NewMetrics(meter)
	if err != nil {
		return err
	}
	svc.Metrics = metrics
	provider, err := flagd.NewProvider()
	if err != nil {
		return err
	}
	if err := openfeature.SetProvider(provider); err != nil {
		return err
	}
	defer openfeature.Shutdown()
	svc.Assigner = policy.FlagAssigner{Client: openfeature.NewClient("emac-checkout-policy")}
	server := &http.Server{Addr: env("LISTEN_ADDR", ":8080"), Handler: svc.Handler(), ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		log.Printf("shutdown on %s", sig)
		flushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(flushCtx)
	case err := <-errCh:
		return err
	}
}
func observability(ctx context.Context) (func(context.Context) error, metric.Meter, error) {
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(env("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://otel-collector:4318/v1/traces")))
	if err != nil {
		return nil, nil, err
	}
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(env("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "http://otel-collector:4318/v1/metrics")))
	if err != nil {
		return nil, nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", "checkout-policy")))
	if err != nil {
		return nil, nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	residualView := sdkmetric.NewView(sdkmetric.Instrument{Name: "emac.policy.residual.duration", Kind: sdkmetric.InstrumentKindHistogram}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000}, NoMinMax: true}})
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second))), sdkmetric.WithResource(res), sdkmetric.WithView(residualView))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	shutdown := func(ctx context.Context) error {
		metricErr := mp.Shutdown(ctx)
		traceErr := tp.Shutdown(ctx)
		if metricErr != nil {
			return metricErr
		}
		return traceErr
	}
	return shutdown, mp.Meter("emac.checkout-policy"), nil
}
func required(name string) string {
	v := os.Getenv(name)
	if v == "" {
		panic(fmt.Sprintf("%s is required", name))
	}
	return v
}
func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
