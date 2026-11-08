package db_test

import (
	"testing"
	"time"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// Teste de integração das queries da saúde dos dados, com dados conhecidos
// no lugar dos reais (só dentro da transação, que sofre rollback no fim).
func TestHealthQueries(t *testing.T) {
	ctx, tx := beginTestTx(t)
	mustExec(t, ctx, tx,
		`DELETE FROM mart.indicator_values`,
		`DELETE FROM mart.fuel_prices_municipal`,
		`DELETE FROM mart.fuel_prices_weekly`,
		`DELETE FROM mart.pipeline_runs`,
		`INSERT INTO mart.indicators (id, name, unit, frequency, source, source_code, source_run_id)
		 VALUES ('usd_brl', 'Dólar', 'R$/US$', 'daily', 'BCB', '1', 7)
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO mart.indicator_values (indicator_id, ref_date, value) VALUES
		   ('usd_brl', '2026-11-05', 5.4621), ('usd_brl', '2026-11-06', 5.5013)`,
		`INSERT INTO mart.fuel_products (id, name, unit) VALUES ('gasolina', 'Gasolina comum', 'R$/l')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO mart.fuel_prices_weekly (product_id, scope, week_start, avg_price, n_stations) VALUES
		   ('gasolina', 'BR', '2026-10-26', 6.505, 4560),
		   ('gasolina', 'SP', '2026-11-02', 6.290, 900)`, // UF não conta: só a média do Brasil
		// anp: uma carga com aviso e depois uma execução sem novidade;
		// bcb: falhou e nunca deu certo.
		`INSERT INTO mart.pipeline_runs
		   (job, started_at, finished_at, status, error, source_run_id, source_collected_at, rows_written, warnings)
		 VALUES
		   ('anp', '2026-11-07 09:00+00', '2026-11-07 09:01+00', 'success', NULL, 49, '2026-11-07 08:00+00', 30985, '["aviso de teste"]'),
		   ('anp', '2026-11-08 09:00+00', '2026-11-08 09:00+00', 'skipped', NULL, NULL, NULL, NULL, '[]'),
		   ('bcb', '2026-11-08 09:00+00', '2026-11-08 09:00+00', 'failed', 'erro de teste', NULL, NULL, NULL, '[]')`,
	)
	q := db.New(tx)

	freshness, err := q.ListDatasetFreshness(ctx)
	if err != nil {
		t.Fatal(err)
	}
	latest := map[string]time.Time{}
	for _, row := range freshness {
		latest[row.Dataset] = row.Latest
	}
	// Sem nenhum IPCA nem Selic, eles simplesmente não aparecem.
	if len(latest) != 2 || !latest["usd_brl"].Equal(d(2026, 11, 6)) || !latest["fuels"].Equal(d(2026, 10, 26)) {
		t.Errorf("freshness = %+v", freshness)
	}

	runs, err := q.ListLastRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Job != "anp" || runs[0].Status != "skipped" || runs[1].Status != "failed" {
		t.Errorf("últimas execuções = %+v", runs)
	}

	ok, err := q.ListLastSuccessfulRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A carga da ANP (com o aviso) aparece mesmo depois do skipped; o bcb não.
	if len(ok) != 1 || ok[0].Job != "anp" || string(ok[0].Warnings) != `["aviso de teste"]` || !ok[0].SourceCollectedAt.Valid {
		t.Errorf("últimas cargas = %+v", ok)
	}
}
