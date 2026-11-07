// Command collector coleta as séries do Banco Central (API SGS) e grava as
// respostas brutas no schema raw. Roda uma vez e termina.
//
// Variáveis de ambiente:
//
//	DATABASE_URL           obrigatória
//	SGS_START_DATE         início das séries diárias (padrão 2000-01-01)
//	COLLECTOR_CONCURRENCY  consultas simultâneas (padrão 4)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	// Embute o banco de fusos horários no binário. A imagem Docker final não
	// tem /usr/share/zoneinfo, e sem isso LoadLocation falharia.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LuizAPLeite/radar-economico/collector/internal/collect"
	"github.com/LuizAPLeite/radar-economico/collector/internal/sgs"
	"github.com/LuizAPLeite/radar-economico/collector/internal/store"
)

type config struct {
	databaseURL string
	startDate   time.Time
	concurrency int
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// main fica pequeno: toda a lógica vai em run, que devolve erro.
	// Isso facilita testar e garante que os defers rodem antes do os.Exit.
	if err := run(logger); err != nil {
		logger.Error("collector terminou com erro", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Ctrl+C (SIGINT) ou `docker stop` (SIGTERM) cancelam o contexto,
	// e as consultas em andamento param de forma limpa.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("conectando ao banco: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("conectando ao banco: %w", err)
	}

	db := store.New(pool)
	runID, err := db.StartRun(ctx, "bcb_sgs")
	if err != nil {
		return err
	}

	today, err := todayInBrazil()
	if err != nil {
		return err
	}
	reqs := collect.BuildRequests(collect.BCBSeries, cfg.startDate, today)
	logger.Info("coleta iniciada", "run_id", runID, "requests", len(reqs),
		"concurrency", cfg.concurrency, "from", cfg.startDate.Format(time.DateOnly), "to", today.Format(time.DateOnly))

	began := time.Now()
	stats, runErr := collect.Run(ctx, logger, sgs.NewClient(logger), db, runID, reqs, cfg.concurrency)

	// Usa um contexto novo: se o anterior foi cancelado por Ctrl+C,
	// ainda queremos registrar que a coleta falhou.
	finishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.FinishRun(finishCtx, runID, runErr); err != nil {
		return errors.Join(runErr, err)
	}

	logger.Info("coleta finalizada", "run_id", runID,
		"saved", stats.Saved, "empty", stats.Empty, "failed", stats.Failed,
		"took_ms", time.Since(began).Milliseconds())
	return runErr
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL: os.Getenv("DATABASE_URL"),
		startDate:   time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		concurrency: 4,
	}
	if cfg.databaseURL == "" {
		return cfg, errors.New("DATABASE_URL não definida")
	}
	if v := os.Getenv("SGS_START_DATE"); v != "" {
		t, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return cfg, fmt.Errorf("SGS_START_DATE inválida (use AAAA-MM-DD): %w", err)
		}
		cfg.startDate = t
	}
	if v := os.Getenv("COLLECTOR_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("COLLECTOR_CONCURRENCY inválida: %q", v)
		}
		cfg.concurrency = n
	}
	return cfg, nil
}

// todayInBrazil devolve a data de hoje em Brasília, à meia-noite UTC, para
// que a janela final não dependa do fuso da máquina que roda o coletor.
func todayInBrazil() (time.Time, error) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.Time{}, fmt.Errorf("carregando fuso horário: %w", err)
	}
	y, m, d := time.Now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
}
