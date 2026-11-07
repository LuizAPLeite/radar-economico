-- staging: observações do SGS já tipadas e validadas, uma linha por série e data.
-- É reconstruída pelo pipeline a cada execução, a partir da coleta
-- bem-sucedida mais recente (que traz o histórico completo).
CREATE TABLE staging.bcb_sgs_observations (
    series_code integer NOT NULL,
    ref_date    date    NOT NULL,
    value       numeric NOT NULL,
    run_id      bigint  NOT NULL, -- coleta de origem (raw.collection_runs.id)
    PRIMARY KEY (series_code, ref_date)
);

-- mart: indicadores prontos para consumo pela API.
CREATE TABLE mart.indicators (
    id            text PRIMARY KEY,      -- identificador estável usado na API: selic, ipca, usd_brl
    name          text        NOT NULL,
    unit          text        NOT NULL,
    frequency     text        NOT NULL CHECK (frequency IN ('daily', 'monthly')),
    source        text        NOT NULL,  -- instituição de origem, ex.: BCB
    source_code   text        NOT NULL,  -- código da série na fonte, ex.: 432
    source_run_id bigint      NOT NULL,  -- coleta que originou os dados atuais
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mart.indicator_values (
    indicator_id text    NOT NULL REFERENCES mart.indicators (id),
    ref_date     date    NOT NULL,
    value        numeric NOT NULL,
    -- A chave primária também serve de índice para a consulta da API:
    -- valores de um indicador num intervalo de datas.
    PRIMARY KEY (indicator_id, ref_date)
);
