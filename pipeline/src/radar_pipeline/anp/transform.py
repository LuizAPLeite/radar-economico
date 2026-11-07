"""Transformações puras (sem banco) dos CSVs de preços da ANP."""

import io
from dataclasses import dataclass

import polars as pl

from radar_pipeline.anp.products import PRODUCTS
from radar_pipeline.anp.schemas import (
    MartFuelPricesMunicipal,
    MartFuelPricesWeekly,
    StagingFuelPrices,
)

# Colunas usadas do CSV (de 16), com os nomes do pipeline. Ler só estas
# reduz bastante a memória: são ~800 mil linhas por ano.
_COLUMNS = {
    "Estado - Sigla": "uf",
    "Municipio": "municipality",
    "CNPJ da Revenda": "cnpj",
    "Produto": "product_anp",
    "Data da Coleta": "collected_on",
    "Valor de Venda": "price",
    "Unidade de Medida": "unit",
    "Bandeira": "brand",
}

# Nomes que a ANP escreve diferente do IBGE mesmo depois de normalizar.
# Chave: (UF, nome como vem da ANP). Valor: código do IBGE.
MUNICIPALITY_EXCEPTIONS: dict[tuple[str, str], int] = {
    ("RS", "SANTANA DO LIVRAMENTO"): 4317103,  # IBGE: Sant'Ana do Livramento
}

_STAGING_COLUMNS = [
    "product",
    "collected_on",
    "cnpj",
    "ibge_code",
    "uf",
    "price",
    "brand",
    "source_file_id",
]


class SourceFormatError(ValueError):
    """Dado da fonte fora do formato esperado (produto novo, unidade, município)."""


def parse_file(content: bytes, file_id: int) -> pl.DataFrame:
    """Lê um CSV da ANP e tipa as colunas.

    Formato (conferido em 2026-11-07): UTF-8 com BOM, separador ";", vírgula
    decimal ("6,89"), datas dd/MM/aaaa e CNPJ formatado, às vezes com espaço
    na frente. As conversões são estritas: um valor fora do formato faz o
    job falhar, em vez de virar nulo em silêncio.
    """
    raw = pl.read_csv(
        io.BytesIO(content), separator=";", columns=list(_COLUMNS), infer_schema=False
    )
    text = [
        pl.col(c).str.strip_chars() for c in ("uf", "municipality", "product_anp", "unit", "brand")
    ]
    return raw.rename(_COLUMNS).with_columns(
        *text,
        pl.col("cnpj").str.replace_all(r"\D", ""),  # só os dígitos
        pl.col("collected_on").str.strip_chars().str.to_date("%d/%m/%Y"),
        pl.col("price").str.strip_chars().str.replace(",", ".", literal=True).cast(pl.Float64),
        pl.lit(file_id, dtype=pl.Int64).alias("source_file_id"),
    )


def normalize_name(name: pl.Expr) -> pl.Expr:
    """Nome de município comparável: sem acentos, maiúsculo, sem hífen nem
    apóstrofo, com espaços simples. "Sant'Ana do Livramento" vira
    "SANT ANA DO LIVRAMENTO"; "São Paulo" vira "SAO PAULO".

    NFKD separa cada letra acentuada em letra + acento; \\p{Mn} é a classe
    Unicode dos acentos soltos, que o replace_all remove.
    """
    return (
        name.str.normalize("NFKD")
        .str.replace_all(r"\p{Mn}", "")
        .str.to_uppercase()
        .str.replace_all(r"[-']", " ")
        .str.replace_all(r"\s+", " ")
        .str.strip_chars()
    )


@dataclass(frozen=True)
class StagingResult:
    frame: pl.DataFrame
    duplicates_removed: int  # linhas repetidas idênticas descartadas


def to_staging(frames: list[pl.DataFrame], municipalities: pl.DataFrame) -> StagingResult:
    """Junta os arquivos, mapeia os produtos, casa os municípios com o IBGE
    e valida. `municipalities` tem as colunas ibge_code, name e uf."""
    prices = _map_products(pl.concat(frames))

    # A fonte repete algumas linhas idênticas (mesmo posto, produto, dia e
    # preço): são cópias, e só uma fica. Linhas com o mesmo posto, produto e
    # dia mas preço DIFERENTE continuam, e o contrato as rejeita.
    before = prices.height
    prices = prices.unique(
        subset=["product", "collected_on", "cnpj", "price"], keep="first", maintain_order=True
    )

    frame = (
        _match_municipalities(prices, municipalities)
        .select(_STAGING_COLUMNS)
        .sort("product", "collected_on", "cnpj")
    )
    return StagingResult(
        frame=StagingFuelPrices.validate(frame, lazy=True),
        duplicates_removed=before - prices.height,
    )


def _map_products(prices: pl.DataFrame) -> pl.DataFrame:
    """Troca o nome da ANP ("DIESEL S10") pelo id da mart ("diesel_s10") e
    confere a unidade. Produto ou unidade desconhecidos fazem o job falhar:
    a ANP mudou algo e o catálogo precisa ser atualizado."""
    catalog = pl.DataFrame(
        {
            "product_anp": [p.anp_name for p in PRODUCTS],
            "product": [p.id for p in PRODUCTS],
            "expected_unit": [p.anp_unit for p in PRODUCTS],
        }
    )
    mapped = prices.join(catalog, on="product_anp", how="left")

    unknown = mapped.filter(pl.col("product").is_null())["product_anp"].unique().sort().to_list()
    if unknown:
        raise SourceFormatError(f"produtos fora do catálogo (anp/products.py): {unknown}")

    wrong_unit = mapped.filter(pl.col("unit") != pl.col("expected_unit"))
    if not wrong_unit.is_empty():
        pairs = wrong_unit.select("product_anp", "unit").unique().sort("product_anp").rows()
        raise SourceFormatError(f"unidades inesperadas: {pairs}")

    return mapped.drop("product_anp", "unit", "expected_unit")


def _match_municipalities(prices: pl.DataFrame, municipalities: pl.DataFrame) -> pl.DataFrame:
    """Acrescenta o código do IBGE de cada linha, casando UF + nome normalizado
    e, para os nomes que não casam, a tabela de exceções."""
    lookup = municipalities.select("uf", "ibge_code", key=normalize_name(pl.col("name")))
    if lookup.select("uf", "key").is_duplicated().any():
        raise SourceFormatError("dois municípios do IBGE com o mesmo nome na mesma UF")

    exceptions = pl.DataFrame(
        [(uf, name, code) for (uf, name), code in MUNICIPALITY_EXCEPTIONS.items()],
        schema={"uf": pl.String, "municipality": pl.String, "exception_code": pl.Int64},
        orient="row",
    )
    matched = (
        prices.with_columns(key=normalize_name(pl.col("municipality")))
        .join(lookup, on=["uf", "key"], how="left")
        .join(exceptions, on=["uf", "municipality"], how="left")
        .with_columns(pl.coalesce("ibge_code", "exception_code").alias("ibge_code"))
    )

    unmatched = matched.filter(pl.col("ibge_code").is_null()).select("uf", "municipality").unique()
    if not unmatched.is_empty():
        names = sorted(unmatched.rows())
        raise SourceFormatError(
            f"municípios sem par no IBGE: {names}. "
            "Inclua-os em MUNICIPALITY_EXCEPTIONS (anp/transform.py) com o código do IBGE."
        )
    return matched


# --- Agregados da mart ----------------------------------------------------------

_AGGREGATES = [
    pl.col("price").mean().round(3).alias("avg_price"),
    pl.col("cnpj").n_unique().cast(pl.Int64).alias("n_stations"),
]
_WEEKLY_COLUMNS = ["product_id", "scope", "week_start", "avg_price", "n_stations"]

# Uma semana com menos postos que esta fração da mediana do produto está
# incompleta. Ex.: sem o arquivo de abril/2026, a semana de 27/04 ficou só
# com 1º e 2/05 (62 postos de gasolina, contra ~4.300 numa semana normal).
MIN_WEEK_COVERAGE = 0.5


@dataclass(frozen=True)
class WeeklyResult:
    frame: pl.DataFrame
    incomplete_weeks: int  # semanas (produto + semana) descartadas


def weekly_averages(staging: pl.DataFrame) -> WeeklyResult:
    """Preço médio semanal de cada produto no Brasil ("BR") e em cada UF.

    A semana começa na segunda-feira (dt.truncate("1w")). A média é simples,
    sobre os preços coletados na semana, e n_stations conta postos distintos.
    Uma semana que cruza a virada do mês junta os dois arquivos mensais.

    Semanas incompletas (ver MIN_WEEK_COVERAGE) saem do Brasil E de todas as
    UFs: a série fica com um buraco honesto, em vez de um ponto calculado com
    uma amostra minúscula, e as UFs continuam comparáveis entre si.
    """
    with_week = staging.with_columns(week_start=pl.col("collected_on").dt.truncate("1w"))
    country = with_week.group_by("product", "week_start").agg(_AGGREGATES)

    # .over("product") calcula a mediana separadamente para cada produto,
    # sem agrupar as linhas (uma "window function", como no SQL).
    threshold = pl.col("n_stations").median().over("product") * MIN_WEEK_COVERAGE
    complete = country.filter(pl.col("n_stations") >= threshold)
    complete_weeks = complete.select("product", "week_start")

    states = (
        with_week.group_by("product", "uf", "week_start")
        .agg(_AGGREGATES)
        .rename({"uf": "scope"})
        # semi join: mantém só as linhas cuja semana também existe em complete_weeks
        .join(complete_weeks, on=["product", "week_start"], how="semi")
    )

    frame = (
        pl.concat([complete.with_columns(scope=pl.lit("BR")), states], how="diagonal")
        .rename({"product": "product_id"})
        .select(_WEEKLY_COLUMNS)
        .sort("product_id", "scope", "week_start")
    )
    return WeeklyResult(
        frame=MartFuelPricesWeekly.validate(frame, lazy=True),
        incomplete_weeks=country.height - complete.height,
    )


def municipal_monthly(staging: pl.DataFrame) -> pl.DataFrame:
    """Preço médio, mínimo e máximo de cada produto por município e mês."""
    frame = (
        staging.with_columns(month=pl.col("collected_on").dt.truncate("1mo"))
        .group_by("product", "month", "ibge_code")
        .agg(
            *_AGGREGATES,
            pl.col("price").min().alias("min_price"),
            pl.col("price").max().alias("max_price"),
        )
        .rename({"product": "product_id"})
        .select(
            "product_id", "month", "ibge_code", "avg_price", "min_price", "max_price", "n_stations"
        )
        .sort("product_id", "month", "ibge_code")
    )
    return MartFuelPricesMunicipal.validate(frame, lazy=True)
