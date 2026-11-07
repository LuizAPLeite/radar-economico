// Command api expõe uma API REST somente leitura sobre o schema mart.
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:    addr,
		Handler: newRouter(),
		// Timeouts explícitos: o http.Server padrão não tem nenhum,
		// o que deixa conexões lentas presas para sempre.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("api ouvindo", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("servidor encerrou com erro", "err", err)
		os.Exit(1)
	}
}

func newRouter() http.Handler {
	mux := http.NewServeMux()
	// Desde o Go 1.22 o ServeMux aceita método no padrão ("GET /health").
	mux.HandleFunc("GET /health", healthHandler)
	return mux
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
