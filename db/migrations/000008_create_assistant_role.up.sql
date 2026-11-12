-- Fase 5: o papel com que a API executa o SQL escrito pelo assistente (LLM).
--
-- O SQL vem de um modelo de linguagem, a partir de uma pergunta de qualquer
-- visitante: ele roda com o MENOR acesso possível. O papel não faz login
-- (NOLOGIN): a API, já conectada, entra nele só durante a consulta
-- (SET LOCAL ROLE, dentro de uma transação) e sai no fim dela. Assim não há
-- senha nova para guardar.
--
-- Só leitura, e só nas tabelas da mart listadas aqui, uma a uma: uma tabela
-- nova não fica visível ao assistente sem uma decisão explícita. Ficam de
-- fora raw, staging e mart.pipeline_runs (que guarda mensagens de erro).
-- Papéis valem para o servidor inteiro, não para um banco: se outro banco do
-- mesmo servidor (um de testes, por exemplo) já criou o papel, ele é reaproveitado.
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'radar_assistant') THEN
        CREATE ROLE radar_assistant NOLOGIN;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA mart TO radar_assistant;
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
TO radar_assistant;

-- O usuário que aplica as migrations é o mesmo da API (no Neon, o dono do
-- banco). Ele recebe o direito de ENTRAR no papel (SET), mas não de herdar
-- as permissões dele (INHERIT FALSE): fora do SET ROLE, nada muda.
GRANT radar_assistant TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
