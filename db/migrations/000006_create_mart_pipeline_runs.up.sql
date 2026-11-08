-- Fase 3: registro das execuções do pipeline, uma linha por job a cada vez
-- que ele roda (inclusive quando falha ou não tem nada novo a processar).
--
-- Fica na mart porque a API lê: o endpoint de saúde dos dados mostra a
-- última execução de cada job. O próprio pipeline também lê, para saber até
-- onde já processou o raw (cargas incrementais).
CREATE TABLE mart.pipeline_runs (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job                 text        NOT NULL CHECK (job <> ''), -- bcb, ibge, anp
    started_at          timestamptz NOT NULL,
    finished_at         timestamptz NOT NULL CHECK (finished_at >= started_at),
    -- skipped: não havia nada no raw para processar.
    status              text        NOT NULL CHECK (status IN ('success', 'skipped', 'failed')),
    -- Mensagem do erro, só nas falhas. É para quem opera o banco: a API não
    -- a expõe.
    error               text,
    -- Coleta mais recente (raw.collection_runs.id) cujos dados o job usou, e
    -- quando ela aconteceu.
    source_run_id       bigint,
    source_collected_at timestamptz,
    rows_written        integer CHECK (rows_written >= 0), -- linhas gravadas na mart
    -- Avisos das checagens de qualidade que não impedem a carga.
    warnings            jsonb       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(warnings) = 'array'),
    CHECK ((status = 'failed') = (error IS NOT NULL))
);

-- A consulta mais comum: a última execução de cada job.
CREATE INDEX pipeline_runs_job_started_idx ON mart.pipeline_runs (job, started_at DESC);
