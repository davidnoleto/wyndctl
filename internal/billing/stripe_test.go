package billing

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/client"
)

// stripeServer is a minimal mock of the two Stripe REST endpoints this package uses.
type stripeServer struct {
	// searchHits records calls to /v1/customers/search. The handler returns
	// the i-th element of searchResponses (clamped to the last) on the i-th call.
	searchHits      atomic.Int32
	searchResponses [][]map[string]any

	// createCalls counts POST /v1/customers.
	createCalls atomic.Int32
}

func (s *stripeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers/search", func(w http.ResponseWriter, r *http.Request) {
		idx := int(s.searchHits.Add(1)) - 1
		if idx >= len(s.searchResponses) {
			idx = len(s.searchResponses) - 1
		}
		data := []map[string]any{}
		if idx >= 0 {
			data = s.searchResponses[idx]
		}
		writeJSON(w, map[string]any{
			"object":   "search_result",
			"url":      "/v1/customers/search",
			"has_more": false,
			"data":     data,
		})
	})
	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.createCalls.Add(1)
		writeJSON(w, map[string]any{
			"id":     "cus_created",
			"object": "customer",
		})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// newTestClient builds a stripe client whose API backend talks to ts.
func newTestClient(ts *httptest.Server) *client.API {
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(ts.URL),
	})
	sc := &client.API{}
	sc.Init("sk_test_dummy", &stripe.Backends{
		API:     backend,
		Uploads: backend,
		Connect: backend,
	})
	return sc
}

// shortenPolling makes pollInterval near-zero for the duration of a test.
func shortenPolling(t *testing.T) {
	t.Helper()
	origInterval, origAttempts := pollInterval, pollMaxAttempts
	pollInterval = time.Millisecond
	pollMaxAttempts = 5
	t.Cleanup(func() {
		pollInterval = origInterval
		pollMaxAttempts = origAttempts
	})
}

func TestFindOrCreateCustomer_AlreadyExists(t *testing.T) {
	srv := &stripeServer{
		searchResponses: [][]map[string]any{
			{{"id": "cus_existing", "object": "customer"}},
		},
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	id, err := findOrCreateCustomer(newTestClient(ts), "alice@example.com", "Alice", "ENTERPRISE", io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "cus_existing" {
		t.Errorf("got id %q, want cus_existing", id)
	}
	if got := srv.createCalls.Load(); got != 0 {
		t.Errorf("expected no create calls, got %d", got)
	}
}

func TestFindOrCreateCustomer_Creates(t *testing.T) {
	shortenPolling(t)

	srv := &stripeServer{
		// 1st search: empty. 2nd: customer appears.
		searchResponses: [][]map[string]any{
			{},
			{{"id": "cus_created", "object": "customer"}},
		},
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	id, err := findOrCreateCustomer(newTestClient(ts), "bob@example.com", "Bob", "ENTERPRISE", io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "cus_created" {
		t.Errorf("got id %q, want cus_created", id)
	}
	if got := srv.createCalls.Load(); got != 1 {
		t.Errorf("expected exactly 1 create call, got %d", got)
	}
}

func TestFindOrCreateCustomer_Timeout(t *testing.T) {
	shortenPolling(t)

	srv := &stripeServer{
		// All searches return empty — the customer never appears.
		searchResponses: [][]map[string]any{{}},
	}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	_, err := findOrCreateCustomer(newTestClient(ts), "carol@example.com", "Carol", "ENTERPRISE", io.Discard)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error %q does not mention timeout", err.Error())
	}
}

func TestFindOrCreateCustomer_EmptyKey(t *testing.T) {
	_, err := FindOrCreateCustomer("", "x@example.com", "X", "ENTERPRISE", nil)
	if err == nil {
		t.Fatal("expected error for empty api key")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("error %q should mention api_key", err.Error())
	}
}
