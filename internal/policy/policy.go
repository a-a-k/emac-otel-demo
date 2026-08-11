package policy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/experiment"
	"github.com/a-a-k/emac-otel-demo/internal/ledger"
	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	HeaderRunID         = "X-Emac-Run-Id"
	HeaderStageID       = "X-Emac-Stage-Id"
	HeaderRequestID     = "X-Emac-Request-Id"
	HeaderEvidenceIndex = "X-Emac-Evidence-Index"
	HeaderRolloutKey    = "X-Emac-Rollout-Key"
	HeaderInternational = "X-Emac-International"
	HeaderPhase         = "X-Emac-Phase"
)

type CartClient interface {
	GetCart(context.Context, *pb.GetCartRequest, ...any) (*pb.Cart, error)
}

// GRPC dependencies use small adapters because generated clients use
// grpc.CallOption rather than ...any.
type CartGetter interface {
	GetCart(context.Context, string) (*pb.Cart, error)
}
type CurrencyGetter interface {
	Supported(context.Context) ([]string, error)
}

type Config struct {
	FrontendURL string
	ShippingURL string
	Weight      float64
	Seeds       experiment.Seeds
	Timeout     time.Duration
}

type Service struct {
	Config   Config
	Cart     CartGetter
	Currency CurrencyGetter
	Client   *http.Client
	Ledger   func(ledger.Request) error
	Metrics  interface {
		Record(context.Context, ledger.Request) error
	}
	Assigner interface {
		Candidate(context.Context, bool, float64) (bool, error)
	}
}

func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

func (s *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		if s.Assigner != nil {
			candidate, err := s.Assigner.Candidate(r.Context(), false, 0)
			if err != nil || candidate {
				http.Error(w, "flag assignment unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.URL.Path != "/api/checkout" {
		http.NotFound(w, r)
		return
	}
	started := time.Now()
	phase := experiment.Phase(r.Header.Get(HeaderPhase))
	ctx := r.Context()
	var root trace.Span
	if phase != experiment.PhaseWarmup {
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
		ctx, root = otel.Tracer("emac.checkout-policy").Start(ctx, "POST /api/checkout", trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
			attribute.String("emac.phase", string(phase)),
			attribute.String("emac.run_id", r.Header.Get(HeaderRunID)),
			attribute.String("emac.stage_id", r.Header.Get(HeaderStageID)),
			attribute.String("emac.request_id", r.Header.Get(HeaderRequestID)),
		))
	}
	meta, err := s.metadata(r)
	if err != nil {
		if root != nil {
			root.RecordError(err)
			root.SetStatus(codes.Error, err.Error())
			root.End()
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if root != nil {
		root.SetAttributes(meta.attributes("POST /api/checkout")...)
	}
	ctx = context.WithValue(ctx, metadataKey{}, meta)
	calls := registeredCalls(meta.branch)
	status, body, rootCorrect := http.StatusBadGateway, []byte(nil), false
	ctx, cancel := context.WithTimeout(ctx, s.Config.Timeout)
	defer cancel()

	if meta.branch == experiment.Candidate {
		cart, callErr := measure(&calls[0], func() (*pb.Cart, error) { return s.Cart.GetCart(ctx, meta.userID) })
		if callErr == nil && validCart(cart) {
			currencies, currencyErr := measure(&calls[1], func() ([]string, error) { return s.Currency.Supported(ctx) })
			if currencyErr == nil && contains(currencies, "CAD") {
				_, shippingErr := measure(&calls[2], func() ([]byte, error) { return s.shippingQuote(ctx, cart) })
				if shippingErr == nil {
					status, body, rootCorrect = s.frontend(ctx, r, &calls[3])
				}
			}
		}
	} else {
		status, body, rootCorrect = s.frontend(ctx, r, &calls[0])
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	rootDuration := time.Since(started)
	record := ledger.Request{RunID: meta.runID, StageID: meta.stageID, RequestID: meta.requestID, EvidenceIndex: meta.evidenceIndex, EndedAt: time.Now().UTC(), Phase: string(meta.phase), Branch: string(meta.branch), RootCorrect: rootCorrect, Root: rootDuration, Calls: calls}
	if root != nil {
		root.SetAttributes(attribute.Bool("emac.correct", rootCorrect))
		if !rootCorrect {
			root.SetStatus(codes.Error, "invalid checkout response")
		}
		span := root.SpanContext()
		if span.IsValid() {
			record.TraceID = span.TraceID().String()
		}
		root.End()
	}
	if s.Ledger != nil {
		_ = s.Ledger(record)
	}
	if s.Metrics != nil {
		_ = s.Metrics.Record(ctx, record)
	}
}

type requestMetadata struct {
	runID, stageID, requestID, userID string
	evidenceIndex                     int
	evidenceBlock                     string
	phase                             experiment.Phase
	branch                            experiment.Branch
}

type metadataKey struct{}

func (m requestMetadata) attributes(operation string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("emac.operation", operation), attribute.String("emac.run_id", m.runID), attribute.String("emac.stage_id", m.stageID), attribute.String("emac.request_id", m.requestID), attribute.String("emac.phase", string(m.phase)), attribute.String("emac.branch", string(m.branch)), attribute.String("emac.evidence_block", m.evidenceBlock),
	}
	switch operation {
	case "POST /api/checkout", "Frontend/POST api/checkout":
		attrs = append(attrs, attribute.String("http.request.method", "POST"), attribute.String("http.route", "/api/checkout"))
	case "Shipping/POST get-quote":
		attrs = append(attrs, attribute.String("http.request.method", "POST"), attribute.String("http.route", "/get-quote"))
	case "CartService/GetCart":
		attrs = append(attrs, attribute.String("rpc.system", "grpc"), attribute.String("rpc.service", "oteldemo.CartService"), attribute.String("rpc.method", "GetCart"))
	case "CurrencyService/GetSupportedCurrencies":
		attrs = append(attrs, attribute.String("rpc.system", "grpc"), attribute.String("rpc.service", "oteldemo.CurrencyService"), attribute.String("rpc.method", "GetSupportedCurrencies"))
	}
	return attrs
}

func (s *Service) metadata(r *http.Request) (requestMetadata, error) {
	m := requestMetadata{runID: r.Header.Get(HeaderRunID), stageID: r.Header.Get(HeaderStageID), requestID: r.Header.Get(HeaderRequestID), userID: r.URL.Query().Get("userId"), phase: experiment.Phase(r.Header.Get(HeaderPhase))}
	if m.runID == "" || m.stageID == "" || m.requestID == "" || m.userID == "" {
		return m, fmt.Errorf("missing registered identity metadata")
	}
	switch m.phase {
	case experiment.PhaseWarmup, experiment.PhaseMeasured, experiment.PhaseOracle:
	default:
		return m, fmt.Errorf("invalid phase %q", m.phase)
	}
	evidenceIndex, err := strconv.Atoi(r.Header.Get(HeaderEvidenceIndex))
	if err != nil {
		return m, fmt.Errorf("invalid evidence index")
	}
	m.evidenceIndex = evidenceIndex
	m.evidenceBlock = experiment.EvidenceBlock(m.evidenceIndex)
	intl, err := strconv.ParseBool(r.Header.Get(HeaderInternational))
	if err != nil {
		return m, fmt.Errorf("invalid international header")
	}
	bucket := experiment.Bucket(s.Config.Seeds.Rollout, r.Header.Get(HeaderRolloutKey))
	m.branch, err = experiment.Assign(intl, bucket, s.Config.Weight)
	if err == nil && s.Assigner != nil {
		candidate, flagErr := s.Assigner.Candidate(r.Context(), intl, bucket)
		if flagErr != nil {
			return m, flagErr
		}
		if candidate != (m.branch == experiment.Candidate) {
			return m, fmt.Errorf("flagd/local bucket assignment conflict")
		}
	}
	return m, err
}

func registeredCalls(branch experiment.Branch) []ledger.Call {
	if branch != experiment.Candidate {
		return []ledger.Call{{Operation: "Frontend/POST api/checkout", Intended: true}}
	}
	return []ledger.Call{
		{Operation: "CartService/GetCart", Intended: true},
		{Operation: "CurrencyService/GetSupportedCurrencies", Intended: true},
		{Operation: "Shipping/POST get-quote", Intended: true},
		{Operation: "Frontend/POST api/checkout", Intended: true},
	}
}

func measure[T any](call *ledger.Call, fn func() (T, error)) (T, error) {
	call.Attempted = true
	call.SpanCount = 1
	started := time.Now()
	v, err := fn()
	call.Duration = time.Since(started)
	call.Correct = err == nil
	return v, err
}

func (s *Service) shippingQuote(ctx context.Context, cart *pb.Cart) ([]byte, error) {
	return clientSpan(ctx, "Shipping/POST get-quote", func(spanCtx context.Context) ([]byte, error) {
		type item struct {
			ProductID string `json:"product_id"`
			Quantity  int32  `json:"quantity"`
		}
		items := make([]item, 0, len(cart.GetItems()))
		for _, v := range cart.GetItems() {
			items = append(items, item{v.GetProductId(), v.GetQuantity()})
		}
		payload := map[string]any{"items": items, "address": map[string]string{"street_address": "1200 Maple Street", "city": "Toronto", "state": "ON", "country": "Canada", "zip_code": "M4B 1B3"}}
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(spanCtx, http.MethodPost, strings.TrimRight(s.Config.ShippingURL, "/")+"/get-quote", bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		injectHTTP(spanCtx, req)
		resp, err := s.Client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("shipping status %d", resp.StatusCode)
		}
		var decoded struct {
			CostUSD *struct {
				CurrencyCode string `json:"currency_code"`
			} `json:"cost_usd"`
		}
		if json.Unmarshal(body, &decoded) != nil || decoded.CostUSD == nil || decoded.CostUSD.CurrencyCode != "USD" {
			return nil, fmt.Errorf("invalid shipping quote")
		}
		return body, nil
	})
}

type frontendResult struct {
	status int
	body   []byte
}

func (s *Service) frontend(ctx context.Context, original *http.Request, call *ledger.Call) (int, []byte, bool) {
	result, err := measure(call, func() (frontendResult, error) {
		return clientSpan(ctx, "Frontend/POST api/checkout", func(spanCtx context.Context) (frontendResult, error) {
			b, readErr := io.ReadAll(io.LimitReader(original.Body, 1<<20))
			if readErr != nil {
				return frontendResult{status: http.StatusBadGateway}, readErr
			}
			url := strings.TrimRight(s.Config.FrontendURL, "/") + original.URL.RequestURI()
			req, reqErr := http.NewRequestWithContext(spanCtx, http.MethodPost, url, bytes.NewReader(b))
			if reqErr != nil {
				return frontendResult{status: http.StatusBadGateway}, reqErr
			}
			req.Header = original.Header.Clone()
			injectHTTP(spanCtx, req)
			resp, doErr := s.Client.Do(req)
			if doErr != nil {
				return frontendResult{status: http.StatusBadGateway}, doErr
			}
			defer resp.Body.Close()
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			result := frontendResult{resp.StatusCode, body}
			if readErr != nil {
				return result, readErr
			}
			if resp.StatusCode != http.StatusOK || !validCheckout(body) {
				return result, fmt.Errorf("invalid checkout response")
			}
			return result, nil
		})
	})
	if err != nil {
		return result.status, result.body, false
	}
	return result.status, result.body, true
}

func validCheckout(body []byte) bool {
	var v struct {
		OrderID            string `json:"orderId"`
		ShippingTrackingID string `json:"shippingTrackingId"`
		Items              []struct {
			Item struct {
				ProductID string `json:"productId"`
				Quantity  int32  `json:"quantity"`
			} `json:"item"`
		} `json:"items"`
	}
	if json.Unmarshal(body, &v) != nil || v.OrderID == "" || v.ShippingTrackingID == "" || len(v.Items) != 2 {
		return false
	}
	seen := map[string]int32{}
	for _, item := range v.Items {
		seen[item.Item.ProductID] += item.Item.Quantity
	}
	return seen["OLJCESPC7Z"] == 1 && seen["66VCHSJNUP"] == 1 && len(seen) == 2
}

func validCart(cart *pb.Cart) bool {
	if cart == nil || len(cart.GetItems()) != 2 {
		return false
	}
	seen := map[string]int32{}
	for _, item := range cart.GetItems() {
		seen[item.GetProductId()] += item.GetQuantity()
	}
	return seen["OLJCESPC7Z"] == 1 && seen["66VCHSJNUP"] == 1 && len(seen) == 2
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func clientSpan[T any](ctx context.Context, operation string, fn func(context.Context) (T, error)) (T, error) {
	meta, _ := ctx.Value(metadataKey{}).(requestMetadata)
	if meta.phase == experiment.PhaseWarmup {
		return fn(unsampledContext(ctx, meta.requestID+":"+operation))
	}
	spanCtx, span := otel.Tracer("emac.checkout-policy").Start(ctx, operation, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(meta.attributes(operation)...))
	defer span.End()
	v, err := fn(spanCtx)
	span.SetAttributes(attribute.Bool("emac.correct", err == nil))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return v, err
}

func injectHTTP(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

func unsampledContext(ctx context.Context, key string) context.Context {
	h := sha256.Sum256([]byte("emac-unsampled-v1:" + key))
	var tid trace.TraceID
	var sid trace.SpanID
	copy(tid[:], h[:16])
	copy(sid[:], h[16:24])
	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid}))
}

func NewHTTPClient() *http.Client {
	return &http.Client{Transport: http.DefaultTransport, Timeout: 30 * time.Second}
}
