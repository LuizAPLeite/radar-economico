package db_test

import (
	"testing"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// deleteFuelData esvazia (dentro da transação do teste) tudo o que referencia
// mart.fuel_products, na ordem das chaves estrangeiras, e os produtos.
var deleteFuelData = []string{
	`DELETE FROM mart.fuel_forecasts`,
	`DELETE FROM mart.forecast_backtests`,
	`DELETE FROM mart.analysis_rolling_correlations`,
	`DELETE FROM mart.analysis_lag_correlations`,
	`DELETE FROM mart.analysis_regimes`,
	`DELETE FROM mart.fuel_prices_anp_weekly`,
	`DELETE FROM mart.fuel_prices_municipal`,
	`DELETE FROM mart.fuel_prices_weekly`,
	`DELETE FROM mart.fuel_products`,
}

func TestAnalysisQueries(t *testing.T) {
	ctx, tx := beginTestTx(t)
	mustExec(t, ctx, tx, deleteFuelData...)
	mustExec(t, ctx, tx,
		`INSERT INTO mart.fuel_products (id, name, unit) VALUES
		   ('gasolina', 'Gasolina comum', 'R$/l'), ('etanol', 'Etanol hidratado', 'R$/l')`,
		`INSERT INTO mart.analysis_regimes (id, name, starts_on, ends_on, description) VALUES
		   ('atual', 'Atual', '2023-05-16', NULL, 'hoje'),
		   ('ppi', 'Paridade', '2016-10-14', '2023-05-16', 'PPI')`,
		`INSERT INTO mart.analysis_lag_correlations VALUES
		   ('gasolina', 'brent_brl', 'atual', 1, 0.30, 180),
		   ('gasolina', 'brent_brl', 'atual', 0, 0.20, 181),
		   ('etanol',   'brent_brl', 'atual', 0, 0.10, 181)`,
		`INSERT INTO mart.analysis_rolling_correlations VALUES
		   ('gasolina', 'brent_brl', '2026-10-26', 2, 0.38),
		   ('gasolina', 'brent_brl', '2026-10-19', 2, 0.37)`,
		`INSERT INTO mart.fuel_forecasts VALUES
		   ('gasolina', '2026-11-16', 'naive', '2026-11-02', 2, 6.47, 6.42, 6.52),
		   ('gasolina', '2026-11-09', 'naive', '2026-11-02', 1, 6.47, 6.45, 6.49)`,
		`INSERT INTO mart.forecast_backtests VALUES
		   ('gasolina', 'naive', 1, 0.019, 0.3, 104),
		   ('gasolina', 'brent', 1, 0.024, 0.4, 104)`,
		`INSERT INTO mart.fuel_prices_anp_weekly VALUES
		   ('gasolina', '2026-10-26', 6.47, 4300),
		   ('gasolina', '2026-11-02', 6.47, 4310),
		   ('gasolina', '2004-05-10', 2.10, 9000)`,
	)
	q := db.New(tx)

	regimes, err := q.ListAnalysisRegimes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(regimes) != 2 || regimes[0].ID != "ppi" || !regimes[0].EndsOn.Valid || regimes[1].EndsOn.Valid {
		t.Errorf("regimes (por data de início, período atual sem fim) = %+v", regimes)
	}

	lags, err := q.ListLagCorrelations(ctx, "gasolina")
	if err != nil {
		t.Fatal(err)
	}
	if len(lags) != 2 || lags[0].LagWeeks != 0 || lags[1].Correlation != 0.30 || lags[1].NWeeks != 180 {
		t.Errorf("defasagens da gasolina (em ordem de lag) = %+v", lags)
	}

	rolling, err := q.ListRollingCorrelations(ctx, "gasolina")
	if err != nil {
		t.Fatal(err)
	}
	if len(rolling) != 2 || !rolling[0].WeekStart.Equal(d(2026, 10, 19)) {
		t.Errorf("móvel (em ordem de semana) = %+v", rolling)
	}

	forecasts, err := q.ListFuelForecasts(ctx, "gasolina")
	if err != nil {
		t.Fatal(err)
	}
	if len(forecasts) != 2 || forecasts[0].HorizonWeeks != 1 || forecasts[0].Model != "naive" ||
		!forecasts[0].BaseWeek.Equal(d(2026, 11, 2)) || forecasts[0].Lower80 != 6.45 {
		t.Errorf("previsões = %+v", forecasts)
	}

	backtests, err := q.ListForecastBacktests(ctx, "gasolina")
	if err != nil {
		t.Fatal(err)
	}
	if len(backtests) != 2 || backtests[0].Model != "brent" || backtests[1].Mae != 0.019 {
		t.Errorf("backtests (por modelo) = %+v", backtests)
	}

	official, err := q.ListOfficialFuelPrices(ctx, db.ListOfficialFuelPricesParams{
		ProductID: "gasolina", FromDate: d(2026, 1, 1), ToDate: d(2026, 12, 31),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(official) != 2 || official[1].NStations != 4310 {
		t.Errorf("série oficial em 2026 = %+v", official)
	}

	empty, err := q.ListFuelForecasts(ctx, "etanol")
	if err != nil || len(empty) != 0 {
		t.Errorf("etanol sem previsão: %v, %+v", err, empty)
	}
}
