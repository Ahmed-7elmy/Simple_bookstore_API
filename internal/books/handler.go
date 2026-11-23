package books

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bookstore/internal/auth"
)

type Handler struct {
	store Store
	db    *sql.DB
	auth  *auth.Service
}

func NewHandler(store Store, db *sql.DB, authService *auth.Service) http.Handler {
	handler := &Handler{store: store, db: db, auth: authService}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.health)
	mux.Handle("POST /api/v1/auth/login", auth.LoginHandler(authService))
	mux.Handle("GET /api/v1/books", handler.requireRoles([]string{"reader", "admin"}, handler.listBooks))
	mux.Handle("POST /api/v1/books", handler.requireRoles([]string{"admin"}, handler.createBook))
	mux.Handle("GET /api/v1/books/{id}", handler.requireRoles([]string{"reader", "admin"}, handler.getBook))
	mux.Handle("PUT /api/v1/books/{id}", handler.requireRoles([]string{"admin"}, handler.updateBook))
	mux.Handle("DELETE /api/v1/books/{id}", handler.requireRoles([]string{"admin"}, handler.deleteBook))
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "route not found")
	})
	return mux
}

func (h *Handler) requireRoles(roles []string, next http.HandlerFunc) http.Handler {
	return h.auth.RequireRole(roles...)(next)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		slog.Error("database health check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listBooks(w http.ResponseWriter, r *http.Request) {
	books, err := h.store.List(r.Context())
	if err != nil {
		h.internalError(w, "list books", err)
		return
	}
	writeJSON(w, http.StatusOK, books)
}

func (h *Handler) getBook(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	book, err := h.store.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}
	if err != nil {
		h.internalError(w, "get book", err)
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (h *Handler) createBook(w http.ResponseWriter, r *http.Request) {
	book, ok := readBook(w, r)
	if !ok {
		return
	}

	created, err := h.store.Create(r.Context(), book)
	if err != nil {
		h.internalError(w, "create book", err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) updateBook(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	book, ok := readBook(w, r)
	if !ok {
		return
	}

	updated, err := h.store.Update(r.Context(), id, book)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}
	if err != nil {
		h.internalError(w, "update book", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteBook(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.store.Delete(r.Context(), id); errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "book not found")
	} else if err != nil {
		h.internalError(w, "delete book", err)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

type bookInput struct {
	Title           string `json:"title"`
	Author          string `json:"author"`
	PublicationYear int    `json:"publication_year"`
}

func readBook(w http.ResponseWriter, r *http.Request) (Book, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input bookInput
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON with title, author, and publication_year")
		return Book{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return Book{}, false
	}

	input.Title = strings.TrimSpace(input.Title)
	input.Author = strings.TrimSpace(input.Author)
	if input.Title == "" || input.Author == "" || input.PublicationYear < 1 || input.PublicationYear > time.Now().Year() {
		writeError(w, http.StatusBadRequest, "title and author are required; publication_year must be between 1 and the current year")
		return Book{}, false
	}
	return Book{
		Title:           input.Title,
		Author:          input.Author,
		PublicationYear: input.PublicationYear,
	}, true
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "book id must be a positive integer")
		return 0, false
	}
	return id, true
}

func (h *Handler) internalError(w http.ResponseWriter, operation string, err error) {
	slog.Error(operation+" failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write JSON response failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
