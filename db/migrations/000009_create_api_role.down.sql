-- Revoga uma a uma, como no down da 000008: o dono não herda o papel, então
-- DROP OWNED BY não serviria.
REVOKE radar_assistant FROM radar_api;
REVOKE SELECT (id, job, started_at, finished_at, status, source_run_id,
               source_collected_at, rows_written, warnings)
    ON mart.pipeline_runs FROM radar_api;
REVOKE SELECT ON
    mart.indicators,
    mart.indicator_values,
    mart.fuel_products,
    mart.fuel_prices_weekly,
    mart.fuel_prices_municipal,
    mart.fuel_prices_anp_weekly,
    mart.municipalities,
    mart.analysis_regimes,
    mart.analysis_lag_correlations,
    mart.analysis_rolling_correlations,
    mart.fuel_forecasts,
    mart.forecast_backtests
FROM radar_api;
REVOKE USAGE ON SCHEMA mart FROM radar_api;

-- O usuário é do servidor inteiro: se outro banco ainda o usa, ele fica.
DO $$
BEGIN
    DROP ROLE IF EXISTS radar_api;
EXCEPTION WHEN dependent_objects_still_exist THEN
    RAISE NOTICE 'radar_api ainda é usado por outro banco; o usuário fica';
END
$$;
