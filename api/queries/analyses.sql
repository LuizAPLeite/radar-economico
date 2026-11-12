-- name: ListAnalysisRegimes :many
-- Períodos da política de preços usados nas correlações.
SELECT *
  FROM mart.analysis_regimes
 ORDER BY starts_on;

-- name: ListLagCorrelations :many
-- Correlação da variação do preço com a de cada fator, por período e defasagem.
SELECT driver, regime_id, lag_weeks, correlation, n_weeks
  FROM mart.analysis_lag_correlations
 WHERE product_id = $1
 ORDER BY driver, regime_id, lag_weeks;

-- name: ListRollingCorrelations :many
-- Correlação em janela móvel de 52 semanas, na defasagem de cada fator.
SELECT driver, week_start, lag_weeks, correlation
  FROM mart.analysis_rolling_correlations
 WHERE product_id = $1
 ORDER BY driver, week_start;

-- name: ListFuelForecasts :many
-- Previsão mais recente das próximas semanas, com o intervalo de 80%.
SELECT week_start, model, base_week, horizon_weeks, predicted, lower_80, upper_80
  FROM mart.fuel_forecasts
 WHERE product_id = $1
 ORDER BY week_start;

-- name: ListForecastBacktests :many
-- Erro de cada modelo no teste com o passado, por horizonte.
SELECT model, horizon_weeks, mae, mape, n_forecasts
  FROM mart.forecast_backtests
 WHERE product_id = $1
 ORDER BY model, horizon_weeks;

-- name: ListOfficialFuelPrices :many
-- Média semanal oficial da ANP no Brasil (série histórica desde 2004).
SELECT week_start, avg_price, n_stations
  FROM mart.fuel_prices_anp_weekly
 WHERE product_id = sqlc.arg(product_id)
   AND week_start >= sqlc.arg(from_date)
   AND week_start <= sqlc.arg(to_date)
 ORDER BY week_start;
