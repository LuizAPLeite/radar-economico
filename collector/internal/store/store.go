// Package store grava as coletas no schema raw do PostgreSQL.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LuizAPLeite/radar-economico/collector/internal/anp"
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

// BCBCoverage devolve, para cada série do SGS, até que data as coletas
// bem-sucedidas já buscaram: o fim da janela pedida ou, nas consultas sem
// datas (histórico completo), o dia da coleta em Brasília. É o ponto de
// partida da coleta incremental.
func (s *Store) BCBCoverage(ctx context.Context) (map[int]time.Time, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.series_code,
		        max(COALESCE(r.window_end, (c.started_at AT TIME ZONE 'America/Sao_Paulo')::date))
		   FROM raw.bcb_sgs_responses AS r
		   JOIN raw.collection_runs AS c ON c.id = r.run_id
		  WHERE c.status = 'success'
		  GROUP BY r.series_code`)
	if err != nil {
		return nil, fmt.Errorf("store: lendo a cobertura do BCB: %w", err)
	}
	defer rows.Close()

	coverage := make(map[int]time.Time)
	for rows.Next() {
		var (
			code  int
			until time.Time
		)
		if err := rows.Scan(&code, &until); err != nil {
			return nil, fmt.Errorf("store: lendo a cobertura do BCB: %w", err)
		}
		coverage[code] = until
	}
	return coverage, rows.Err()
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

// LatestANPHashes devolve, para cada arquivo da ANP (grupo e mês), o SHA-256
// da versão mais recente guardada.
func (s *Store) LatestANPHashes(ctx context.Context) (map[anp.Key]string, error) {
	// DISTINCT ON fica com a primeira linha de cada (grupo, mês) segundo o
	// ORDER BY: a mais recente.
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT ON (product_group, ref_month) product_group, ref_month, sha256
		   FROM raw.anp_files
		  ORDER BY product_group, ref_month, fetched_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: lendo hashes da ANP: %w", err)
	}
	defer rows.Close()

	latest := make(map[anp.Key]string)
	for rows.Next() {
		var (
			group, sha string
			month      time.Time
		)
		if err := rows.Scan(&group, &month, &sha); err != nil {
			return nil, fmt.Errorf("store: lendo hashes da ANP: %w", err)
		}
		latest[anp.File{Group: group, Month: month}.Key()] = sha
	}
	return latest, rows.Err()
}

// SaveANPFile grava um arquivo da ANP já compactado.
func (s *Store) SaveANPFile(ctx context.Context, runID int64, f anp.File, sha256 string, size int64, gz []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO raw.anp_files (run_id, product_group, ref_month, url, sha256, size_bytes, content_gzip)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		runID, f.Group, f.Month, f.URL, sha256, size, gz,
	)
	if err != nil {
		return fmt.Errorf("store: gravando arquivo da ANP %s %s: %w", f.Group, f.Key().Month, err)
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
