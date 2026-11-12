package assistant

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Integração: precisa de um Postgres com as migrations (o papel
// radar_assistant vem da 000008). Sem TEST_DATABASE_URL, é pulado.
func newTestExecutor(t *testing.T) (*PgExecutor, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definida; pulando teste de integração")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1 // uma conexão só: dá para ver se ela volta limpa ao pool
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &PgExecutor{Pool: pool}, pool
}

func TestPgExecutor_TypesAndValues(t *testing.T) {
	e, _ := newTestExecutor(t)
	res, err := e.Run(context.Background(), `
		SELECT date '2026-11-02' AS semana, 6.473::numeric AS preco, 42 AS postos,
		       'SP'::text AS uf, NULL::numeric AS vazio, 0.5::float8 AS fator`)
	if err != nil {
		t.Fatal(err)
	}
	wantCols := []Column{
		{"semana", "date"}, {"preco", "number"}, {"postos", "number"},
		{"uf", "text"}, {"vazio", "number"}, {"fator", "number"},
	}
	for i, c := range wantCols {
		if res.Columns[i] != c {
			t.Errorf("coluna %d = %+v, quer %+v", i, res.Columns[i], c)
		}
	}
	row := res.Rows[0]
	if row[0] != "2026-11-02" || row[1] != 6.473 || row[2] != float64(42) || row[3] != "SP" || row[4] != nil || row[5] != 0.5 {
		t.Errorf("linha = %#v", row)
	}
	if res.Truncated {
		t.Error("não deveria estar cortado")
	}
}

func TestPgExecutor_LimitsRows(t *testing.T) {
	e, _ := newTestExecutor(t)
	res, err := e.Run(context.Background(), "SELECT n FROM generate_series(1, 600) AS n")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != MaxRows || !res.Truncated {
		t.Errorf("%d linhas, cortado = %v; quer %d e true", len(res.Rows), res.Truncated, MaxRows)
	}

	// Um WITH também vira subconsulta e respeita o limite.
	res, err = e.Run(context.Background(), "WITH x AS (SELECT n FROM generate_series(1, 3) n) SELECT * FROM x")
	if err != nil || len(res.Rows) != 3 || res.Truncated {
		t.Errorf("WITH: %v, %d linhas", err, len(res.Rows))
	}
}

func TestPgExecutor_Blocks(t *testing.T) {
	e, _ := newTestExecutor(t)
	tests := []struct {
		name, sql, want string
	}{
		{"registro de execuções", "SELECT * FROM mart.pipeline_runs", "permission denied"},
		{"schema raw", "SELECT * FROM raw.collection_runs", "permission denied"},
		{"schema staging", "SELECT * FROM staging.fuel_prices", "permission denied"},
		{"consulta demorada", "SELECT pg_sleep(5)", "statement timeout"},
		// CheckSQL já barra isto; se passasse, a transação só de leitura barraria.
		{"escrita", "SELECT 1 FROM (DELETE FROM mart.fuel_products RETURNING 1) AS d", "syntax error"},
		{"função que escreve", "SELECT nextval('mart.pipeline_runs_id_seq')", "permission denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.Run(context.Background(), tt.sql)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("erro = %v, quer algo com %q", err, tt.want)
			}
		})
	}
}

func TestPgExecutor_ConnectionComesBackClean(t *testing.T) {
	e, pool := newTestExecutor(t)
	ctx := context.Background()
	var before string
	if err := pool.QueryRow(ctx, "SELECT current_user").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(ctx, "SELECT current_user AS usuario"); err != nil {
		t.Fatal(err)
	}
	// A mesma (única) conexão do pool, depois da consulta do assistente.
	var after, timeout string
	if err := pool.QueryRow(ctx, "SELECT current_user, current_setting('statement_timeout')").Scan(&after, &timeout); err != nil {
		t.Fatal(err)
	}
	if after != before || timeout == queryTimeout {
		t.Errorf("depois: usuário %q (antes %q), timeout %q: o SET LOCAL vazou da transação", after, before, timeout)
	}
}
