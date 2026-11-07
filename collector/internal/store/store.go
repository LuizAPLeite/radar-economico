// Package store grava as coletas no schema raw do PostgreSQL.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LuizAPLeite/radar-economico/collector/internal/sgs"
)

// Store guarda um pool de conexões. O pgxpool é seguro para uso por várias
// goroutines ao mesmo tempo: cada consulta pega uma conexão livre e devolve.
type Store struct {
	pool *pgxpool.Pool
}

// New cria um Store sobre um pool já conectado.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// StartRun registra o início de uma coleta e devolve o id dela.
func (s *Store) StartRun(ctx context.Context, source string) (int64, error) {
	var id int64
	// Os parâmetros $1, $2... são enviados separados do SQL: nunca concatene
	// valores na string, isso abre espaço para SQL injection.
	err := s.pool.QueryRow(ctx,
		`INSERT INTO raw.collection_runs (source) VALUES ($1) RETURNING id`,
		source,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: iniciando coleta: %w", err)
	}
	return id, nil
}

// FinishRun marca a coleta como concluída, com sucesso ou com o erro dado.
func (s *Store) FinishRun(ctx context.Context, runID int64, runErr error) error {
	status, msg := "success", (*string)(nil)
	if runErr != nil {
		status = "failed"
		m := runErr.Error()
		msg = &m // ponteiro nil vira NULL no banco
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE raw.collection_runs
		    SET finished_at = now(), status = $2, error = $3
		  WHERE id = $1`,
		runID, status, msg,
	)
	if err != nil {
		return fmt.Errorf("store: finalizando coleta %d: %w", runID, err)
	}
	return nil
}

// SaveResponse grava uma resposta da API SGS exatamente como veio.
func (s *Store) SaveResponse(ctx context.Context, runID int64, r sgs.Response) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO raw.bcb_sgs_responses
		     (run_id, series_code, window_start, window_end, request_url, http_status, payload)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		runID, r.Request.SeriesCode, dateOrNil(r.Request.Start), dateOrNil(r.Request.End),
		r.URL, r.StatusCode, string(r.Body),
	)
	if err != nil {
		return fmt.Errorf("store: gravando série %d: %w", r.Request.SeriesCode, err)
	}
	return nil
}

// dateOrNil converte a data zero (consulta sem filtro) em NULL.
func dateOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
