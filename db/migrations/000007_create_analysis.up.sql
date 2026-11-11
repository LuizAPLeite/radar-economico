-- Fase 4: fontes para as análises (série histórica da ANP e Brent) e os
-- resultados das análises (repasse para a bomba e previsão de preços).

-- Arquivos brutos de fontes que publicam um arquivo inteiro (planilha, CSV),
-- como vieram, compactados com gzip. Mesmo padrão de raw.anp_files: o SHA-256
-- é do conteúdo original, e um arquivo republicado sem mudança não é guardado
-- de novo.
CREATE TABLE raw.source_files (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id       bigint      NOT NULL REFERENCES raw.collection_runs (id),
    source       text        NOT NULL CHECK (source <> ''), -- ex.: anp_serie, fred
    name         text        NOT NULL CHECK (name <> ''),   -- ex.: semanal-brasil-desde-2013, DCOILBRENTEU
    url          text        NOT NULL,
    sha256       text        NOT NULL,
    size_bytes   bigint      NOT NULL, -- tamanho original, antes do gzip
    content_gzip bytea       NOT NULL,
    fetched_at   timestamptz NOT NULL DEFAULT now()
);

-- Já chega compactado: não vale a pena o Postgres tentar comprimir de novo.
ALTER TABLE raw.source_files ALTER COLUMN content_gzip SET STORAGE EXTERNAL;

-- Versão mais recente de cada arquivo de uma fonte.
CREATE INDEX source_files_source_name_idx
    ON raw.source_files (source, name, fetched_at DESC);

-- mart: preço médio semanal no Brasil segundo a própria ANP (série histórica
-- do levantamento de preços, desde 2004). A ANP começa a semana no domingo;
-- aqui ela vai para a segunda seguinte, como em mart.fuel_prices_weekly.
CREATE TABLE mart.fuel_prices_anp_weekly (
    product_id text    NOT NULL REFERENCES mart.fuel_products (id),
    week_start date    NOT NULL CHECK (extract(isodow FROM week_start) = 1),
    avg_price  numeric NOT NULL CHECK (avg_price > 0),
    n_stations integer NOT NULL CHECK (n_stations > 0),
    PRIMARY KEY (product_id, week_start)
);

-- mart: períodos da política de preços dos combustíveis, usados para separar
-- as análises (a relação com o petróleo muda conforme a regra da Petrobras).
CREATE TABLE mart.analysis_regimes (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    starts_on   date NOT NULL,
    ends_on     date, -- NULL: período atual
    description text NOT NULL,
    CHECK (ends_on IS NULL OR ends_on > starts_on)
);

-- mart: correlação entre a variação do preço na bomba e a variação de um
-- fator (dólar, Brent em US$, Brent em R$) algumas semanas antes.
CREATE TABLE mart.analysis_lag_correlations (
    product_id  text             NOT NULL REFERENCES mart.fuel_products (id),
    driver      text             NOT NULL CHECK (driver IN ('usd_brl', 'brent_usd', 'brent_brl')),
    regime_id   text             NOT NULL REFERENCES mart.analysis_regimes (id),
    lag_weeks   integer          NOT NULL CHECK (lag_weeks >= 0),
    correlation double precision NOT NULL CHECK (correlation BETWEEN -1 AND 1),
    n_weeks     integer          NOT NULL CHECK (n_weeks > 0),
    PRIMARY KEY (product_id, driver, regime_id, lag_weeks)
);

-- mart: correlação em janela móvel (ex.: 52 semanas), para ver a relação
-- mudar ao longo do tempo. week_start é a última semana da janela.
CREATE TABLE mart.analysis_rolling_correlations (
    product_id  text             NOT NULL REFERENCES mart.fuel_products (id),
    driver      text             NOT NULL CHECK (driver IN ('usd_brl', 'brent_usd', 'brent_brl')),
    week_start  date             NOT NULL CHECK (extract(isodow FROM week_start) = 1),
    lag_weeks   integer          NOT NULL CHECK (lag_weeks >= 0),
    correlation double precision NOT NULL CHECK (correlation BETWEEN -1 AND 1),
    PRIMARY KEY (product_id, driver, week_start)
);

-- mart: previsão mais recente do preço médio semanal no Brasil, com o
-- intervalo de 80%. base_week é a última semana observada quando foi feita.
CREATE TABLE mart.fuel_forecasts (
    product_id    text    NOT NULL REFERENCES mart.fuel_products (id),
    week_start    date    NOT NULL CHECK (extract(isodow FROM week_start) = 1),
    base_week     date    NOT NULL CHECK (base_week < week_start),
    horizon_weeks integer NOT NULL CHECK (horizon_weeks > 0),
    predicted     numeric NOT NULL,
    lower_80      numeric NOT NULL,
    upper_80      numeric NOT NULL,
    PRIMARY KEY (product_id, week_start),
    CHECK (lower_80 <= predicted AND predicted <= upper_80)
);

-- mart: erro de cada modelo no teste com o passado (backtest), por horizonte.
-- naive = "o preço fica igual"; os outros modelos precisam vencer este.
CREATE TABLE mart.forecast_backtests (
    product_id    text             NOT NULL REFERENCES mart.fuel_products (id),
    model         text             NOT NULL CHECK (model <> ''),
    horizon_weeks integer          NOT NULL CHECK (horizon_weeks > 0),
    mae           double precision NOT NULL CHECK (mae >= 0), -- erro absoluto médio, R$
    mape          double precision NOT NULL CHECK (mape >= 0), -- erro percentual médio, %
    n_forecasts   integer          NOT NULL CHECK (n_forecasts > 0),
    PRIMARY KEY (product_id, model, horizon_weeks)
);
