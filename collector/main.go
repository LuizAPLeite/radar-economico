// Command collector coleta as fontes de dados (Banco Central, IBGE, ANP) e
// grava o conteúdo bruto no schema raw. Roda uma vez e termina.
//
// Variáveis de ambiente:
//
//	DATABASE_URL           obrigatória
//	COLLECTOR_SOURCES      fontes a coletar, separadas por vírgula (padrão: todas)
//	SGS_START_DATE         início das séries diárias do BCB (padrão 2000-01-01)
//	COLLECTOR_CONCURRENCY  consultas simultâneas ao BCB (padrão 4)
//	ANP_START_MONTH        primeiro mês da ANP, AAAA-MM (padrão: 12 meses atrás)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	// Embute o banco de fusos horários no binário. A imagem Docker final não
	// tem /usr/share/zoneinfo, e sem isso LoadLocation falharia.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LuizAPLeite/radar-economico/collector/internal/anp"
	"github.com/LuizAPLeite/radar-economico/collector/internal/collect"
	"github.com/LuizAPLeite/radar-economico/collector/internal/fetch"
	"github.com/LuizAPLeite/radar-economico/collector/internal/sgs"
	"github.com/LuizAPLeite/radar-economico/collector/internal/store"
)

type config struct {
	databaseURL string
	sources     []string // vazio = todas
	startDate   time.Time
	concurrency int
	anpStart    time.Time // zero = padrão (ver defaultANPStart)
}

// source é uma fonte de dados: o nome (gravado em raw.collection_runs.source)
// e a função que faz a coleta.
type source struct {
	name    string
	collect func(ctx context.Context, runID int64) error
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
	// e as coletas em andamento param de forma limpa.
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

	today, err := todayInBrazil()
	if err != nil {
		return err
	}
	if cfg.anpStart.IsZero() {
		cfg.anpStart = defaultANPStart(today)
	}

	db := store.New(pool)
	sources, err := selectSources(buildSources(cfg, logger, db, today), cfg.sources)
	if err != nil {
		return err
	}
	return runSources(ctx, logger, db, sources)
}

// buildSources lista as fontes que o collector sabe coletar.
func buildSources(cfg config, logger *slog.Logger, db *store.Store, today time.Time) []source {
	return []source{
		{"bcb_sgs", func(ctx context.Context, runID int64) error {
			reqs := collect.BuildRequests(collect.BCBSeries, cfg.startDate, today)
			stats, err := collect.BCB(ctx, logger, sgs.NewClient(logger), db, runID, reqs, cfg.concurrency)
			logger.Info("resumo do bcb_sgs", "requests", len(reqs),
				"saved", stats.Saved, "empty", stats.Empty, "failed", stats.Failed)
			return err
		}},
		{"ibge", func(ctx context.Context, runID int64) error {
			return collect.IBGE(ctx, logger, fetch.NewClient(logger, time.Minute), db, runID, collect.IBGEDatasets)
		}},
		{"anp", func(ctx context.Context, runID int64) error {
			// O servidor da ANP é lento (~200 KB/s): até 5 min por arquivo e
			// só 2 downloads por vez, para não sobrecarregá-lo.
			stats, err := collect.ANP(ctx, logger, fetch.NewClient(logger, 5*time.Minute), db, runID, collect.ANPConfig{
				PageURL:     anp.DatasetPageURL,
				StartMonth:  cfg.anpStart,
				Recheck:     2,
				Concurrency: 2,
				Discover:    anp.Discover,
			})
			logger.Info("resumo da anp", "from", cfg.anpStart.Format("2006-01"),
				"found", stats.Found, "downloaded", stats.Downloaded, "saved", stats.Saved,
				"unchanged", stats.Unchanged, "failed", stats.Failed, "missing", stats.Missing)
			return err
		}},
	}
}

// defaultANPStart devolve o 1º dia do mês 12 meses antes do mês de hoje: a
// janela padrão cobre um ano de preços. (O Go normaliza meses fora de 1..12:
// janeiro - 12 vira janeiro do ano anterior.)
func defaultANPStart(today time.Time) time.Time {
	return time.Date(today.Year(), today.Month()-12, 1, 0, 0, 0, 0, time.UTC)
}

// selectSources filtra as fontes pelos nomes pedidos (vazio = todas).
func selectSources(all []source, names []string) ([]source, error) {
	if len(names) == 0 {
		return all, nil
	}
	var selected []source
	for _, name := range names {
		i := slices.IndexFunc(all, func(s source) bool { return s.name == name })
		if i == -1 {
			valid := make([]string, len(all))
			for k, s := range all {
				valid[k] = s.name
			}
			return nil, fmt.Errorf("COLLECTOR_SOURCES: fonte %q desconhecida (válidas: %s)", name, strings.Join(valid, ", "))
		}
		selected = append(selected, all[i])
	}
	return selected, nil
}

// runSources coleta as fontes ao mesmo tempo, cada uma numa goroutine. Uma
// falha não interrompe as outras; os erros voltam juntos no final.
func runSources(ctx context.Context, logger *slog.Logger, db *store.Store, sources []source) error {
	// errs[i] só é escrito pela goroutine i: posições diferentes do slice,
	// então não há disputa e não precisa de mutex.
	errs := make([]error, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Go(func() { errs[i] = runSource(ctx, logger, db, src) })
	}
	wg.Wait()
	return errors.Join(errs...)
}

// runSource registra a coleta de uma fonte em raw.collection_runs (início,
// fim e status) e executa a coleta.
func runSource(ctx context.Context, logger *slog.Logger, db *store.Store, src source) error {
	runID, err := db.StartRun(ctx, src.name)
	if err != nil {
		return fmt.Errorf("%s: %w", src.name, err)
	}
	logger.Info("coleta iniciada", "source", src.name, "run_id", runID)

	began := time.Now()
	runErr := src.collect(ctx, runID)

	// Usa um contexto novo: se o anterior foi cancelado por Ctrl+C,
	// ainda queremos registrar que a coleta falhou.
	finishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.FinishRun(finishCtx, runID, runErr); err != nil {
		return fmt.Errorf("%s: %w", src.name, errors.Join(runErr, err))
	}

	logger.Info("coleta finalizada", "source", src.name, "run_id", runID,
		"ok", runErr == nil, "took_ms", time.Since(began).Milliseconds())
	if runErr != nil {
		return fmt.Errorf("%s: %w", src.name, runErr)
	}
	return nil
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
	if v := os.Getenv("COLLECTOR_SOURCES"); v != "" {
		for name := range strings.SplitSeq(v, ",") {
			if name = strings.TrimSpace(name); name != "" && !slices.Contains(cfg.sources, name) {
				cfg.sources = append(cfg.sources, name)
			}
		}
	}
	if v := os.Getenv("SGS_START_DATE"); v != "" {
		t, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return cfg, fmt.Errorf("SGS_START_DATE inválida (use AAAA-MM-DD): %w", err)
		}
		cfg.startDate = t
	}
	if v := os.Getenv("ANP_START_MONTH"); v != "" {
		t, err := time.Parse("2006-01", v)
		if err != nil {
			return cfg, fmt.Errorf("ANP_START_MONTH inválida (use AAAA-MM): %w", err)
		}
		cfg.anpStart = t
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
