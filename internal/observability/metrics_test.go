package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsUseRoutePatternsInsteadOfIDs(t *testing.T) {
	metrics := NewMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/books/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := metrics.Middleware(mux)

	for _, path := range []string{"/api/v1/books/1", "/api/v1/books/2"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	output := string(body)

	if !strings.Contains(output, `route="GET /api/v1/books/{id}",status="204"} 2`) {
		t.Fatalf("expected two requests for the ID-free route pattern, got:\n%s", output)
	}
	if strings.Contains(output, `route="GET /api/v1/books/1"`) || strings.Contains(output, `route="GET /api/v1/books/2"`) {
		t.Fatalf("metrics should not include book IDs as labels:\n%s", output)
	}
}
