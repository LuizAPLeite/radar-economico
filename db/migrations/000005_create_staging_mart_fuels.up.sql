-- Fase 2: municípios (IBGE) e preços de combustíveis (ANP), limpos e prontos.

-- staging: municípios do IBGE, com o centroide (coordenadas para o mapa).
CREATE TABLE staging.municipalities (
    ibge_code integer PRIMARY KEY,
    name      text    NOT NULL,
    uf        text    NOT NULL CHECK (uf ~ '^[A-Z]{2}$'),
    latitude  double precision, -- NULL quando o IBGE não publica o centroide
    longitude double precision
);

-- staging: um preço por posto, produto e data de coleta, já tipado e com o
-- município casado com o código do IBGE.
CREATE TABLE staging.fuel_prices (
    product        text    NOT NULL,
    collected_on   date    NOT NULL,
    cnpj           text    NOT NULL CHECK (cnpj ~ '^[0-9]{14}$'),
    ibge_code      integer NOT NULL,
    uf             text    NOT NULL CHECK (uf ~ '^[A-Z]{2}$'),
    price          numeric NOT NULL,
    brand          text    NOT NULL, -- bandeira do posto ("BRANCA" = sem bandeira)
    source_file_id bigint  NOT NULL, -- raw.anp_files.id de onde a linha veio
    PRIMARY KEY (product, collected_on, cnpj)
);

-- mart: municípios com coordenadas, para o mapa.
CREATE TABLE mart.municipalities (
    ibge_code integer PRIMARY KEY,
    name      text             NOT NULL,
    uf        text             NOT NULL,
    latitude  double precision NOT NULL,
    longitude double precision NOT NULL
);

-- mart: combustíveis publicados.
CREATE TABLE mart.fuel_products (
    id         text PRIMARY KEY, -- gasolina, gasolina_aditivada, etanol, diesel, diesel_s10, gnv
    name       text        NOT NULL,
    unit       text        NOT NULL, -- R$/l ou R$/m³
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- mart: preço médio semanal por produto, no Brasil ('BR') e em cada UF.
CREATE TABLE mart.fuel_prices_weekly (
    product_id text    NOT NULL REFERENCES mart.fuel_products (id),
    scope      text    NOT NULL CHECK (scope ~ '^[A-Z]{2}$'), -- 'BR' ou a sigla da UF
    week_start date    NOT NULL CHECK (extract(isodow FROM week_start) = 1), -- segunda-feira
    avg_price  numeric NOT NULL,
    n_stations integer NOT NULL CHECK (n_stations > 0),
    PRIMARY KEY (product_id, scope, week_start)
);

-- mart: preço médio mensal por produto e município, para o mapa.
CREATE TABLE mart.fuel_prices_municipal (
    product_id text    NOT NULL REFERENCES mart.fuel_products (id),
    month      date    NOT NULL CHECK (extract(day FROM month) = 1),
    ibge_code  integer NOT NULL REFERENCES mart.municipalities (ibge_code),
    avg_price  numeric NOT NULL,
    min_price  numeric NOT NULL,
    max_price  numeric NOT NULL,
    n_stations integer NOT NULL CHECK (n_stations > 0),
    PRIMARY KEY (product_id, month, ibge_code)
);
