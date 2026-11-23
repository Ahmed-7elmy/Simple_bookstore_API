package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(
		"test-secret-that-is-long-enough-for-hmac",
		[]User{
			{Username: "admin", Password: "admin-pass", Role: "admin"},
			{Username: "reader", Password: "reader-pass", Role: "reader"},
		},
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestLoginAndRoleAuthorization(t *testing.T) {
	service := testService(t)
	readerToken, _, err := service.Login("reader", "reader-pass")
	if err != nil {
		t.Fatalf("login reader: %v", err)
	}

	handler := service.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims.Subject != "reader" {
			t.Errorf("claims in context = %+v, want reader", claims)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/books/1", nil)
	request.Header.Set("Authorization", "Bearer "+readerToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestMissingTokenIsUnauthorized(t *testing.T) {
	handler := testService(t).RequireRole("reader")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler should not run")
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/books", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestLoginHandlerRejectsInvalidCredentials(t *testing.T) {
	handler := LoginHandler(testService(t))
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"wrong"}`),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
