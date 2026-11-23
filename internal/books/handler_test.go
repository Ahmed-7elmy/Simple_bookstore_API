package books

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bookstore/internal/auth"
)

type memoryStore struct {
	books  map[int64]Book
	nextID int64
}

func newMemoryStore() *memoryStore {
	return &memoryStore{books: make(map[int64]Book), nextID: 1}
}

func newTestHandler(t *testing.T, store Store, username string) (http.Handler, string) {
	t.Helper()
	authService, err := auth.NewService(
		"test-secret-that-is-long-enough-for-hmac",
		[]auth.User{
			{Username: "admin", Password: "admin-pass", Role: "admin"},
			{Username: "reader", Password: "reader-pass", Role: "reader"},
		},
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authService.Login(username, username+"-pass")
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(store, nil, authService), token
}

func authorizedRequest(method, target, token, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func (s *memoryStore) List(context.Context) ([]Book, error) {
	result := make([]Book, 0, len(s.books))
	for _, book := range s.books {
		result = append(result, book)
	}
	return result, nil
}

func (s *memoryStore) Get(_ context.Context, id int64) (Book, error) {
	book, ok := s.books[id]
	if !ok {
		return Book{}, ErrNotFound
	}
	return book, nil
}

func (s *memoryStore) Create(_ context.Context, book Book) (Book, error) {
	book.ID = s.nextID
	book.CreatedAt = time.Now()
	s.nextID++
	s.books[book.ID] = book
	return book, nil
}

func (s *memoryStore) Update(_ context.Context, id int64, book Book) (Book, error) {
	if _, ok := s.books[id]; !ok {
		return Book{}, ErrNotFound
	}
	book.ID = id
	book.CreatedAt = s.books[id].CreatedAt
	s.books[id] = book
	return book, nil
}

func (s *memoryStore) Delete(_ context.Context, id int64) error {
	if _, ok := s.books[id]; !ok {
		return ErrNotFound
	}
	delete(s.books, id)
	return nil
}

func TestBookCRUD(t *testing.T) {
	store := newMemoryStore()
	handler, token := newTestHandler(t, store, "admin")

	create := authorizedRequest(http.MethodPost, "/api/v1/books", token,
		`{"title":"The Go Programming Language","author":"Alan A. A. Donovan","publication_year":2015}`)
	createdResponse := httptest.NewRecorder()
	handler.ServeHTTP(createdResponse, create)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body: %s", createdResponse.Code, http.StatusCreated, createdResponse.Body)
	}

	var created Book
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created book: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("created id = %d, want 1", created.ID)
	}

	get := authorizedRequest(http.MethodGet, "/api/v1/books/1", token, "")
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getResponse.Code, http.StatusOK)
	}

	update := authorizedRequest(http.MethodPut, "/api/v1/books/1", token,
		`{"title":"The Go Programming Language (2nd edition)","author":"Alan A. A. Donovan","publication_year":2015}`)
	updateResponse := httptest.NewRecorder()
	handler.ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateResponse.Code, http.StatusOK)
	}

	deleteRequest := authorizedRequest(http.MethodDelete, "/api/v1/books/1", token, "")
	deleteResponse := httptest.NewRecorder()
	handler.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteResponse.Code, http.StatusNoContent)
	}
}

func TestCreateBookRejectsInvalidInput(t *testing.T) {
	handler, token := newTestHandler(t, newMemoryStore(), "admin")
	request := authorizedRequest(http.MethodPost, "/api/v1/books", token,
		`{"title":"","author":"Author","publication_year":2020}`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestReaderCannotCreateBooks(t *testing.T) {
	handler, token := newTestHandler(t, newMemoryStore(), "reader")
	request := authorizedRequest(http.MethodPost, "/api/v1/books", token,
		`{"title":"Book","author":"Author","publication_year":2020}`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestGetMissingBookReturnsNotFound(t *testing.T) {
	handler, token := newTestHandler(t, newMemoryStore(), "reader")
	request := authorizedRequest(http.MethodGet, "/api/v1/books/10", token, "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestDeleteMissingBookReturnsNotFound(t *testing.T) {
	handler, token := newTestHandler(t, newMemoryStore(), "admin")
	request := authorizedRequest(http.MethodDelete, "/api/v1/books/10", token, "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
