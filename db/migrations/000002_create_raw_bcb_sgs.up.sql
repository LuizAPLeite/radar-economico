-- Cada execução do collector gera uma linha aqui. Serve para agrupar as
-- respostas de uma mesma coleta e, mais tarde, para monitorar a saúde dos dados.
CREATE TABLE raw.collection_runs (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source      text        NOT NULL,
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status      text        NOT NULL DEFAULT 'running'
                CHECK (status IN ('running', 'success', 'failed')),
    error       text
);

-- Respostas da API SGS do Banco Central, exatamente como vieram.
-- Append-only: o collector só insere; quem interpreta é o pipeline.
CREATE TABLE raw.bcb_sgs_responses (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id       bigint      NOT NULL REFERENCES raw.collection_runs (id),
    series_code  integer     NOT NULL,
    window_start date,                 -- NULL quando a consulta não usa datas
    window_end   date,
    request_url  text        NOT NULL,
    http_status  integer     NOT NULL, -- 200, ou 404 quando o período não tem dados
    payload      jsonb       NOT NULL, -- corpo da resposta, sem alteração
    fetched_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX bcb_sgs_responses_series_fetched_idx
    ON raw.bcb_sgs_responses (series_code, fetched_at DESC);
