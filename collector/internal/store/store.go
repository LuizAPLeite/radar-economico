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

// SaveIBGE grava uma resposta do IBGE, mas só se ela mudou desde a última
// gravação do mesmo conjunto (compara o SHA-256). Devolve se gravou.
//
// A comparação e a gravação estão no MESMO comando SQL: não há janela entre
// "ler o último hash" e "inserir" em que outra coleta pudesse se intrometer.
func (s *Store) SaveIBGE(ctx context.Context, runID int64, dataset, url, sha256 string, payload []byte) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO raw.ibge_responses (run_id, dataset, request_url, sha256, payload)
		 SELECT $1, $2, $3, $4, $5::jsonb
		  WHERE $4 IS DISTINCT FROM (
		        SELECT sha256 FROM raw.ibge_responses
		         WHERE dataset = $2
		         ORDER BY fetched_at DESC, id DESC
		         LIMIT 1)`,
		runID, dataset, url, sha256, string(payload),
	)
	if err != nil {
		return false, fmt.Errorf("store: gravando ibge %s: %w", dataset, err)
	}
	return tag.RowsAffected() == 1, nil
}

// dateOrNil converte a data zero (consulta sem filtro) em NULL.
func dateOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
