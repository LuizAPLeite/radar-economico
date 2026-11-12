-- As permissões são revogadas uma a uma, pelo dono das tabelas. DROP OWNED
-- BY não serve aqui: ele exige ter os privilégios do papel, e o dono entra
-- nele sem herdá-los (INHERIT FALSE), como no Neon, onde não é superusuário.
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
FROM radar_assistant;
REVOKE USAGE ON SCHEMA mart FROM radar_assistant;
DROP ROLE radar_assistant;
