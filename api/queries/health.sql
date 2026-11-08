-- name: ListDatasetFreshness :many
-- Data do dado mais recente de cada conjunto que o site mostra: um por
-- indicador e um para os combustíveis (a média nacional semanal). Um
-- conjunto sem nenhum dado simplesmente não aparece; a API o trata como
-- ausente.
SELECT indicator_id AS dataset, max(ref_date)::date AS latest
  FROM mart.indicator_values
 GROUP BY indicator_id
UNION ALL
SELECT 'fuels', max(week_start)::date
  FROM mart.fuel_prices_weekly
 WHERE scope = 'BR'
HAVING count(*) > 0;

-- name: ListLastRuns :many
-- A execução mais recente de cada job do pipeline, de qualquer tipo.
SELECT DISTINCT ON (job) job, status, finished_at
  FROM mart.pipeline_runs
 ORDER BY job, started_at DESC;

-- name: ListLastSuccessfulRuns :many
-- A carga mais recente que deu certo em cada job, com os avisos dela. Depois
-- de uma carga com avisos, as execuções sem novidade (skipped) não podem
-- escondê-los.
SELECT DISTINCT ON (job) job, finished_at, source_collected_at, warnings
  FROM mart.pipeline_runs
 WHERE status = 'success'
 ORDER BY job, started_at DESC;
