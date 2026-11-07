package db_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// beginTestTx abre uma transação num Postgres de verdade para um teste de
// integração. Ela nunca é confirmada: o Rollback no fim do teste devolve o
// banco exatamente ao estado anterior. Sem TEST_DATABASE_URL, o teste é pulado.
func beginTestTx(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definida; pulando teste de integração")
	}
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// t.Cleanup roda na ordem inversa do registro: primeiro o rollback, depois o close.
	t.Cleanup(func() { _ = conn.Close(ctx) })
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return ctx, tx
}

// mustExec executa comandos de preparação e interrompe o teste no primeiro erro.
func mustExec(t *testing.T, ctx context.Context, tx pgx.Tx, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparando dados: %v\n%s", err, stmt)
		}
	}
}

// Teste de integração das queries de indicadores geradas pelo sqlc.
func TestQueries(t *testing.T) {
	ctx, tx := beginTestTx(t)

	// Começa de uma mart vazia (só dentro da transação) e insere dados conhecidos.
	for _, stmt := range []string{
		`DELETE FROM mart.indicator_values`,
		`DELETE FROM mart.indicators`,
		`INSERT INTO mart.indicators (id, name, unit, frequency, source, source_code, source_run_id, updated_at)
		 VALUES ('usd_brl', 'Dólar', 'R$/US$', 'daily',   'BCB', '1',   7, '2026-11-07 18:39:18+00'),
		        ('ipca',    'IPCA',  '% a.m.', 'monthly', 'BCB', '433', 7, '2026-11-07 18:39:18+00')`,
		`INSERT INTO mart.indicator_values (indicator_id, ref_date, value)
		 VALUES ('usd_brl', '2026-11-04', 5.4778), ('usd_brl', '2026-11-05', 5.4621),
		        ('usd_brl', '2026-11-06', 5.5013), ('ipca', '2026-09-01', 0.82)`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparando dados: %v", err)
		}
	}

	// db.New aceita qualquer coisa que execute SQL: pool, conexão ou transação.
	q := db.New(tx)

	t.Run("ListIndicators traz o valor mais recente de cada indicador", func(t *testing.T) {
		rows, err := q.ListIndicators(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("len = %d, quer 2", len(rows))
		}
		ipca, usd := rows[0], rows[1] // ordenado por id
		if ipca.MartIndicator.ID != "ipca" || ipca.LastValue != 0.82 || ipca.LastDate.Format(time.DateOnly) != "2026-09-01" {
			t.Errorf("ipca = %+v", ipca)
		}
		if usd.MartIndicator.ID != "usd_brl" || usd.LastValue != 5.5013 || usd.LastDate.Format(time.DateOnly) != "2026-11-06" {
			t.Errorf("usd_brl = %+v", usd)
		}
		if !usd.MartIndicator.UpdatedAt.Equal(time.Date(2026, 11, 7, 18, 39, 18, 0, time.UTC)) {
			t.Errorf("updated_at = %v", usd.MartIndicator.UpdatedAt)
		}
	})

	t.Run("GetIndicator traz o indicador completo", func(t *testing.T) {
		ind, err := q.GetIndicator(ctx, "usd_brl")
		if err != nil {
			t.Fatal(err)
		}
		if ind.Unit != "R$/US$" || ind.SourceCode != "1" || ind.SourceRunID != 7 {
			t.Errorf("indicador = %+v", ind)
		}
	})

	t.Run("GetIndicator inexistente devolve pgx.ErrNoRows", func(t *testing.T) {
		_, err := q.GetIndicator(ctx, "nao_existe")
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, quer pgx.ErrNoRows", err)
		}
	})

	t.Run("ListIndicatorValues respeita o intervalo fechado", func(t *testing.T) {
		rows, err := q.ListIndicatorValues(ctx, db.ListIndicatorValuesParams{
			IndicatorID: "usd_brl",
			FromDate:    time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
			ToDate:      time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].Value != 5.4621 || rows[1].Value != 5.5013 {
			t.Fatalf("rows = %+v", rows)
		}
	})
}
