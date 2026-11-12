// Package httpapi expõe a mart por uma API REST somente leitura.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter monta as rotas e os middlewares da API.
func NewRouter(store Store, logger *slog.Logger, openAPISpec []byte) http.Handler {
	h := &handlers{store: store, logger: logger}

	r := chi.NewRouter()

	// Middleware é uma função que embrulha um http.Handler em outro: roda
	// código antes e depois da rota. Eles se aplicam na ordem do r.Use.
	r.Use(middleware.RequestID)   // um id por requisição, para cruzar com os logs
	r.Use(requestLogger(logger))  // uma linha de log por requisição
	r.Use(middleware.Recoverer)   // um panic vira 500, em vez de derrubar o servidor
	r.Use(middleware.Compress(5)) // gzip quando o cliente aceita (a série toda cai de ~400 KB para ~40 KB)

	// Até os erros de roteamento respondem no formato JSON padrão da API.
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "rota não encontrada")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "método não permitido")
	})

	r.Get("/health", h.health)
	r.Get("/health/data", h.healthData)
	r.Get("/openapi.yaml", serveSpec(openAPISpec))
	r.Route("/v1", func(r chi.Router) {
		r.Get("/indicators", h.listIndicators)
		r.Get("/indicators/{id}/values", h.listIndicatorValues)
		r.Get("/fuels", h.listFuels)
		r.Get("/fuels/{id}/prices", h.listFuelPrices)
		r.Get("/fuels/{id}/municipalities", h.listFuelMunicipalities)
		r.Get("/fuels/{id}/pass-through", h.getPassThrough)
		r.Get("/fuels/{id}/forecast", h.getForecast)
	})
	return r
}

// requestLogger registra método, rota, status e duração de cada requisição.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// O http.ResponseWriter não conta o status que foi escrito;
			// o wrapper do chi guarda essa informação para o log.
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"took_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

// serveSpec publica a especificação OpenAPI, embutida no binário.
func serveSpec(spec []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(spec)
	}
}
