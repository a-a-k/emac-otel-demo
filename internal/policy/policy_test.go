package policy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/experiment"
	"github.com/a-a-k/emac-otel-demo/internal/ledger"
	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
)

func TestValidCheckout(t *testing.T) {
	if !validCheckout([]byte(`{"orderId":"o","shippingTrackingId":"t","items":[{"item":{"productId":"OLJCESPC7Z","quantity":1}},{"item":{"productId":"66VCHSJNUP","quantity":1}}]}`)) {
		t.Fatal("valid response rejected")
	}
	if validCheckout([]byte(`{"orderId":"o","shippingTrackingId":"","items":[{},{}]}`)) {
		t.Fatal("invalid response accepted")
	}
}

type fakeCart struct{ items []*pb.CartItem }

func (f fakeCart) GetCart(context.Context, string) (*pb.Cart, error) {
	return &pb.Cart{Items: f.items}, nil
}

type fakeCurrency struct{}

func (fakeCurrency) Supported(context.Context) ([]string, error) { return []string{"USD", "CAD"}, nil }

type fakeAssigner bool

func (f fakeAssigner) Candidate(context.Context, bool, float64) (bool, error) { return bool(f), nil }

func TestCandidateJourneyAndLedger(t *testing.T) {
	shipping := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get-quote" {
			t.Errorf("shipping path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"cost_usd":{"currency_code":"USD","units":1,"nanos":0}}`)
	}))
	defer shipping.Close()
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"orderId":"o","shippingTrackingId":"t","items":[{"item":{"productId":"OLJCESPC7Z","quantity":1}},{"item":{"productId":"66VCHSJNUP","quantity":1}}]}`)
	}))
	defer frontend.Close()
	seeds := experiment.DeriveSeeds([]byte("seed"))
	rollout := ""
	for i := 0; i < 1000; i++ {
		rk, _, _ := experiment.Identity(seeds, "r", "10", uint64(i))
		if experiment.Bucket(seeds.Rollout, rk) < .1 {
			rollout = rk
			break
		}
	}
	if rollout == "" {
		t.Fatal("no candidate rollout key")
	}
	var got ledger.Request
	svc := Service{Config: Config{FrontendURL: frontend.URL, ShippingURL: shipping.URL, Weight: .1, Seeds: seeds, Timeout: time.Second}, Cart: fakeCart{[]*pb.CartItem{{ProductId: "OLJCESPC7Z", Quantity: 1}, {ProductId: "66VCHSJNUP", Quantity: 1}}}, Currency: fakeCurrency{}, Client: NewHTTPClient(), Assigner: fakeAssigner(true), Ledger: func(r ledger.Request) error { got = r; return nil }}
	req := httptest.NewRequest(http.MethodPost, "http://policy/api/checkout?userId=u&currencyCode=CAD", strings.NewReader(`{}`))
	req.Header.Set(HeaderRunID, "r")
	req.Header.Set(HeaderStageID, "10")
	req.Header.Set(HeaderRequestID, "q")
	req.Header.Set(HeaderRolloutKey, rollout)
	req.Header.Set(HeaderInternational, "true")
	req.Header.Set(HeaderPhase, "measured")
	response := httptest.NewRecorder()
	svc.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got.Branch != string(experiment.Candidate) || len(got.Calls) != 4 {
		t.Fatalf("ledger %#v", got)
	}
	for _, call := range got.Calls {
		if !call.Attempted || !call.Correct || call.SpanCount != 1 {
			t.Fatalf("call %#v", call)
		}
	}
	if _, _, err := ledger.ValidateAndProject(got, time.Microsecond); err != nil {
		t.Fatal(err)
	}
}

func TestAssignmentConflictFailsClosed(t *testing.T) {
	seeds := experiment.DeriveSeeds([]byte("seed"))
	svc := Service{Config: Config{Weight: 0, Seeds: seeds}, Assigner: fakeAssigner(true)}
	req := httptest.NewRequest(http.MethodPost, "http://policy/api/checkout?userId=u", nil)
	req.Header.Set(HeaderRunID, "r")
	req.Header.Set(HeaderStageID, "10")
	req.Header.Set(HeaderRequestID, "q")
	req.Header.Set(HeaderRolloutKey, "rk")
	req.Header.Set(HeaderInternational, "true")
	req.Header.Set(HeaderPhase, "measured")
	response := httptest.NewRecorder()
	svc.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("got %d", response.Code)
	}
}

func TestHealthChecksFlagAssignment(t *testing.T) {
	svc := Service{Assigner: fakeAssigner(false)}
	response := httptest.NewRecorder()
	svc.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://policy/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("got %d", response.Code)
	}
}
