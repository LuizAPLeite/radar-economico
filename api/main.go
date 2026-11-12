// Command api expõe uma API REST somente leitura sobre o schema mart.
//
// Variáveis de ambiente:
//
//	DATABASE_URL  obrigatória
//	PORT          porta HTTP (padrão 8080)
//	CACHE_TTL     por quanto tempo guardar em memória o que vem da mart,
//	              como 30m ou 1h (padrão 30m; 0 desliga o cache)
//	GEMINI_API_KEY  chave da API do Gemini; sem ela, o assistente (POST
//	              /v1/ask) fica desligado e responde 503
//	GEMINI_MODEL  modelo do Gemini (padrão gemini-3.5-flash-lite)
//	ASSISTANT_DAILY_LIMIT  perguntas novas por dia, abaixo da cota grátis
//	              do Gemini (padrão 400: o 3.5-flash-lite grátis permite 500
//	              por dia e 15 por minuto; respostas guardadas não contam)
package main

import (
	"context"
	_ "embed" // necessário para o //go:embed abaixo
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/LuizAPLeite/radar-economico/api/internal/assistant"
	"github.com/LuizAPLeite/radar-economico/api/internal/cachedstore"
	"github.com/LuizAPLeite/radar-economico/api/internal/db"
	"github.com/LuizAPLeite/radar-economico/api/internal/httpapi"
	"github.com/LuizAPLeite/radar-economico/api/internal/postgres"
)

// go:embed copia o arquivo para dentro do binário na compilação: a spec é
// servida em /openapi.yaml sem precisar acompanhar o executável.
//
//go:embed openapi.yaml
var openAPISpec []byte

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("api encerrou com erro", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL não definida")
	}
	addr := ":" + envOr("PORT", "8080")
	cacheTTL, err := durationEnv("CACHE_TTL", 30*time.Minute)
	if err != nil {
		return err
	}

	// Ctrl+C ou SIGTERM (docker stop, deploy no Render) cancelam o contexto.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Os handlers recebem um httpapi.Store: o banco direto ou o banco
	// embrulhado pelo cache. Os dois têm os mesmos métodos, então o resto do
	// código não muda.
	var store httpapi.Store = db.New(pool)
	if cacheTTL > 0 {
		store = cachedstore.New(store, cacheTTL)
		logger.Info("cache em memória ligado", "ttl", cacheTTL.String())
	} else {
		logger.Info("cache em memória desligado (CACHE_TTL=0)")
	}

	var opts []httpapi.Option
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		limit, err := intEnv("ASSISTANT_DAILY_LIMIT", 400)
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation("America/Sao_Paulo")
		if err != nil {
			return err
		}
		model := envOr("GEMINI_MODEL", "gemini-3.5-flash-lite")
		today := func() string { return time.Now().In(loc).Format(time.DateOnly) }
		svc := assistant.NewService(
			assistant.NewGeminiPlanner(key, model, today),
			&assistant.PgExecutor{Pool: pool},
			logger, limit, loc,
		)
		opts = append(opts, httpapi.WithAssistant(svc))
		logger.Info("assistente ligado", "model", model, "daily_limit", limit)
	} else {
		logger.Info("assistente desligado (sem GEMINI_API_KEY)")
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: httpapi.NewRouter(store, logger, openAPISpec, opts...),
		// Timeouts explícitos: o http.Server padrão não tem nenhum,
		// o que deixa conexões lentas presas para sempre.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Uma pergunta ao assistente espera o LLM (até ~45 s); o resto
		// responde em milissegundos.
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ListenAndServe bloqueia até o servidor parar, então roda numa goroutine.
	// O canal leva o erro de volta (ex.: porta já em uso).
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api ouvindo", "addr", addr)
		serveErr <- srv.ListenAndServe()
	}()

	// select espera o que acontecer primeiro: o servidor falhar ou um sinal de parada.
	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	// Encerramento gracioso: para de aceitar conexões novas e espera as
	// requisições em andamento terminarem (até 10 s).
	logger.Info("encerrando a api")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("api encerrada")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// intEnv lê um inteiro positivo.
func intEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s inválida (%q): use um número inteiro positivo", key, v)
	}
	return n, nil
}

// durationEnv lê uma duração no formato do time.ParseDuration ("30m", "1h30m", "0").
func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s inválida (%q): use uma duração como 30m ou 1h, ou 0 para desligar", key, v)
	}
	return d, nil
}
