-- Segurança (DevSecOps, achado AM-01 do modelo de ameaças): a API deixa de
-- conectar como dono do banco e passa a usar o usuário radar_api, que só lê
-- a mart.
--
-- Por quê: o papel radar_assistant (000008) restringe o SQL do assistente,
-- mas o Postgres sempre deixa uma sessão voltar ao próprio usuário dela
-- (set_config('role', ...) dentro de um SELECT). Com a API conectada como
-- dono, voltar ao dono abria o banco inteiro. Conectada como radar_api,
-- voltar a ela dá acesso só ao que a API já lê.
--
-- A senha NÃO fica aqui: o usuário nasce sem login, e quem administra o banco
-- define a senha fora do git (ALTER ROLE radar_api LOGIN PASSWORD '...'). No
-- Docker Compose, o serviço db-users faz isso com uma senha de desenvolvimento.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'radar_api') THEN
        CREATE ROLE radar_api NOLOGIN;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA mart TO radar_api;
GRANT SELECT ON
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
TO radar_api;

-- O registro das execuções alimenta o /health/data, mas a coluna error (a
-- mensagem interna de cada falha) fica de fora: permissão por coluna.
GRANT SELECT (id, job, started_at, finished_at, status, source_run_id,
              source_collected_at, rows_written, warnings)
    ON mart.pipeline_runs TO radar_api;

-- Como o dono antes, a API entra no papel do assistente só durante a consulta
-- dele (SET LOCAL ROLE), sem herdar as permissões dele fora disso.
GRANT radar_assistant TO radar_api WITH INHERIT FALSE, SET TRUE;
