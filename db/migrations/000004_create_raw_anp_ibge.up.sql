-- Fase 2: arquivos de preços da ANP e dados de municípios do IBGE, como vieram.

-- Um arquivo CSV da série histórica mensal da ANP (preços por posto) por linha.
-- O conteúdo é guardado INTACTO, só compactado com gzip (~4x menor). O SHA-256
-- é do conteúdo original: serve para conferir a integridade e para a coleta
-- incremental (um arquivo republicado sem mudanças não é guardado de novo).
CREATE TABLE raw.anp_files (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id        bigint      NOT NULL REFERENCES raw.collection_runs (id),
    product_group text        NOT NULL CHECK (product_group IN ('gasolina-etanol', 'diesel-gnv')),
    ref_month     date        NOT NULL CHECK (extract(day FROM ref_month) = 1), -- 1º dia do mês
    url           text        NOT NULL,
    sha256        text        NOT NULL,
    size_bytes    bigint      NOT NULL, -- tamanho original, antes do gzip
    content_gzip  bytea       NOT NULL,
    fetched_at    timestamptz NOT NULL DEFAULT now()
);

-- O conteúdo já chega compactado: tentar comprimir de novo (o padrão do
-- Postgres para valores grandes) só gastaria CPU. EXTERNAL guarda sem comprimir.
ALTER TABLE raw.anp_files ALTER COLUMN content_gzip SET STORAGE EXTERNAL;

-- Para a coleta incremental: qual é a versão mais recente de cada grupo e mês?
CREATE INDEX anp_files_group_month_idx
    ON raw.anp_files (product_group, ref_month, fetched_at DESC);

-- Respostas da API do IBGE (lista de municípios e centroides), como vieram.
CREATE TABLE raw.ibge_responses (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id      bigint      NOT NULL REFERENCES raw.collection_runs (id),
    dataset     text        NOT NULL CHECK (dataset IN ('municipios', 'centroides')),
    request_url text        NOT NULL,
    sha256      text        NOT NULL, -- do corpo original, para não gravar repetido
    payload     jsonb       NOT NULL,
    fetched_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ibge_responses_dataset_idx
    ON raw.ibge_responses (dataset, fetched_at DESC);
