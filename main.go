package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bookstore/internal/auth"
	"bookstore/internal/books"
	"bookstore/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://bookstore:bookstore_dev@localhost:5432/bookstore?sslmode=disable"
	}

	tracingShutdown, err := observability.SetupTracing(context.Background(), os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracingShutdown(shutdownCtx); err != nil {
			slog.Error("flush traces failed", "error", err)
		}
	}()

	db, err := books.OpenDatabase(databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	tokenService, err := auth.NewService(envOrDefault("JWT_SECRET", "local-bookstore-jwt-secret-change-this-before-use"),
		[]auth.User{
			{
				Username: envOrDefault("ADMIN_USERNAME", "admin"),
				Password: envOrDefault("ADMIN_PASSWORD", "admin_dev"),
				Role:     "admin",
			},
			{
				Username: envOrDefault("READER_USERNAME", "reader"),
				Password: envOrDefault("READER_PASSWORD", "reader_dev"),
				Role:     "reader",
			},
		},
		time.Hour,
	)
	if err != nil {
		return fmt.Errorf("configure authentication: %w", err)
	}

	store := books.NewPostgresStore(db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	metrics := observability.NewMetrics()
	router := http.NewServeMux()
	router.Handle("/metrics", metrics.Handler())
	router.Handle("/", books.NewHandler(store, db, tokenService))

	server := &http.Server{
		Addr: ":8080",
		Handler: otelhttp.NewHandler(
			metrics.Middleware(observability.LoggingMiddleware(router)),
			"bookstore-api",
		),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("starting bookstore API", "address", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}

	return nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
