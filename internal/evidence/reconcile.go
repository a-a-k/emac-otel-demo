package evidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/a-a-k/emac-otel-demo/internal/experiment"
	"github.com/a-a-k/emac-otel-demo/internal/ledger"
)

type Reconciliation struct {
	Valid            bool           `json:"valid"`
	Proportion       float64        `json:"proportion"`
	ExpectedRoots    int            `json:"expected_roots"`
	ObservedRoots    int            `json:"observed_roots"`
	ExpectedEdges    map[string]int `json:"expected_edges"`
	ObservedEdges    map[string]int `json:"observed_edges"`
	ExpectedSentinel int            `json:"expected_sentinel"`
	ObservedSentinel int            `json:"observed_sentinel"`
	Conflicts        []string       `json:"conflicts"`
}

func Reconcile(ledgerPath, beringDir string, proportion float64) (Reconciliation, error) {
	r := Reconciliation{Proportion: proportion, ExpectedEdges: map[string]int{}, ObservedEdges: map[string]int{}}
	file, err := os.Open(ledgerPath)
	if err != nil {
		return r, err
	}
	defer file.Close()
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var req ledger.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			return r, err
		}
		if req.Phase != "measured" {
			continue
		}
		if req.RequestID == "" || seen[req.RequestID] {
			r.Conflicts = append(r.Conflicts, "missing or duplicate request_id "+req.RequestID)
			continue
		}
		seen[req.RequestID] = true
		if _, _, err := ledger.ValidateAndProject(req, time.Microsecond); err != nil {
			r.Conflicts = append(r.Conflicts, err.Error())
		}
		if !experiment.Sample(req.TraceID, proportion) {
			continue
		}
		r.ExpectedRoots++
		for _, call := range req.Calls {
			if call.Attempted {
				r.ExpectedEdges[call.Operation]++
			}
		}
		if req.Branch == string(experiment.Candidate) {
			r.ExpectedSentinel++
		}
	}
	if err := scanner.Err(); err != nil {
		return r, err
	}
	windows, err := LoadArchive(beringDir)
	if err != nil {
		return r, err
	}
	for _, w := range windows {
		if !w.Available || w.Snapshot == nil {
			continue
		}
		r.ObservedRoots += w.Snapshot.Ingest.Traces
		for _, edge := range w.Snapshot.Discovery.Edges {
			r.ObservedEdges[normalizedOperation(edge)] += edge.Support.TraceCount
		}
	}
	for op, want := range r.ExpectedEdges {
		if got := r.ObservedEdges[op]; got != want {
			r.Conflicts = append(r.Conflicts, fmt.Sprintf("%s trace_count got %d want %d", op, got, want))
		}
	}
	r.ObservedSentinel = r.ObservedEdges["CartService/GetCart"]
	if r.ObservedRoots != r.ExpectedRoots {
		r.Conflicts = append(r.Conflicts, fmt.Sprintf("root trace_count got %d want %d", r.ObservedRoots, r.ExpectedRoots))
	}
	if r.ObservedSentinel != r.ExpectedSentinel {
		r.Conflicts = append(r.Conflicts, fmt.Sprintf("sentinel trace_count got %d want %d", r.ObservedSentinel, r.ExpectedSentinel))
	}
	r.Valid = len(r.Conflicts) == 0
	return r, nil
}

func normalizedOperation(e Edge) string {
	method := e.Identity["rpc.method"]
	service := e.Identity["rpc.service"]
	if method == "" {
		// Bering's operation-aware identity keeps rpc.method in operation, but
		// does not promise to retain rpc.service. Resolve the service from the
		// admitted runtime edge rather than depending on an optional attribute.
		method = e.Identity["operation"]
	}
	if method != "" {
		if service == "oteldemo.CartService" || (e.To == "cart" && method == "GetCart") {
			return "CartService/" + method
		}
		if service == "oteldemo.CurrencyService" || (e.To == "currency" && method == "GetSupportedCurrencies") {
			return "CurrencyService/" + method
		}
	}
	if method == "POST" {
		route := e.Identity["route"]
		if route == "/get-quote" {
			return "Shipping/POST get-quote"
		}
		if route == "/api/checkout" {
			return "Frontend/POST api/checkout"
		}
	}
	return method
}
