package evidence

import "testing"

func TestNormalizedOperationUsesRuntimeEdgeForGRPC(t *testing.T) {
	tests := []struct {
		edge Edge
		want string
	}{
		{Edge{To: "cart", Identity: map[string]string{"operation": "GetCart"}}, "CartService/GetCart"},
		{Edge{To: "currency", Identity: map[string]string{"operation": "GetSupportedCurrencies"}}, "CurrencyService/GetSupportedCurrencies"},
		{Edge{To: "shipping", Identity: map[string]string{"operation": "POST", "route": "/get-quote"}}, "Shipping/POST get-quote"},
		{Edge{To: "frontend", Identity: map[string]string{"operation": "POST", "route": "/api/checkout"}}, "Frontend/POST api/checkout"},
	}
	for _, test := range tests {
		if got := normalizedOperation(test.edge); got != test.want {
			t.Errorf("normalizedOperation(%v) = %q, want %q", test.edge, got, test.want)
		}
	}
}
