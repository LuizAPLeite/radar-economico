"""Transformações puras (sem banco) das planilhas da série histórica da ANP."""

from datetime import timedelta

import fastexcel
import pandera.polars as pa
import polars as pl
from pandera.polars import PolarsData

from radar_pipeline.anp.products import PRODUCT_IDS, PRODUCTS
from radar_pipeline.anp.transform import SourceFormatError
from radar_pipeline.text import br_number

# Primeira célula da linha de cabeçalho. Antes dela, a planilha tem um texto
# com observações sobre a pesquisa, de tamanho variável: o cabeçalho é
# procurado, e não lido de uma linha fixa.
_HEADER_MARKER = "DATA INICIAL"

# Colunas usadas (de 16), com os nomes do pipeline.
_COLUMNS = {
    "DATA INICIAL": "week_start_anp",
    "PRODUTO": "product_anp",
    "NÚMERO DE POSTOS PESQUISADOS": "n_stations",
    "UNIDADE DE MEDIDA": "unit",
    "PREÇO MÉDIO REVENDA": "avg_price",
}

# Nome do produto na série histórica → id da mart. Até 2012 a ANP escrevia
# "ÓLEO DIESEL" (o diesel comum da época), depois "OLEO DIESEL" (S500).
SERIES_PRODUCTS: dict[str, str] = {
    "GASOLINA COMUM": "gasolina",
    "GASOLINA ADITIVADA": "gasolina_aditivada",
    "ETANOL HIDRATADO": "etanol",
    "ÓLEO DIESEL": "diesel",
    "OLEO DIESEL": "diesel",
    "OLEO DIESEL S10": "diesel_s10",
    "GNV": "gnv",
}

# Produtos da planilha que o dashboard não publica.
IGNORED_PRODUCTS = {"GLP"}

# Unidade esperada de cada produto, depois de normalizar "m³" para "m3"
# (a ANP usa as duas grafias).
_UNITS = {p.id: p.unit.replace("³", "3") for p in PRODUCTS}

# Limite inferior dos preços históricos: em 2004 o diesel custava ~R$ 1,50 e
# o GNV ~R$ 1,00, abaixo dos mínimos do catálogo (calibrados em 2025-2026).
HISTORY_MIN_PRICE = 0.5


def parse_sheet(content: bytes) -> pl.DataFrame:
    """Lê uma planilha da série semanal e devolve as colunas usadas, tipadas.

    Formato (conferido em 2026-11-11): uma aba; observações no topo; depois o
    cabeçalho e uma linha por produto e semana. Datas como data do Excel e
    números como número. Coluna faltando ou com outro nome faz o job falhar.
    """
    reader = fastexcel.read_excel(content)
    first_column = reader.load_sheet(0, header_row=None, use_columns=[0]).to_polars()
    matches = (
        first_column.with_row_index()
        .filter(pl.col(first_column.columns[0]) == _HEADER_MARKER)["index"]
        .to_list()
    )
    if not matches:
        raise SourceFormatError(f"cabeçalho ({_HEADER_MARKER!r}) não encontrado na planilha")

    sheet = reader.load_sheet(0, header_row=matches[0]).to_polars()
    missing = _COLUMNS.keys() - set(sheet.columns)
    if missing:
        raise SourceFormatError(f"colunas faltando na planilha da ANP: {sorted(missing)}")
    return sheet.select(
        pl.col(source).alias(target) for source, target in _COLUMNS.items()
    ).with_columns(
        pl.col("week_start_anp").cast(pl.Date, strict=True),
        pl.col("n_stations").cast(pl.Int64, strict=True),
        pl.col("avg_price").cast(pl.Float64, strict=True),
    )


def to_mart(sheets: list[pl.DataFrame]) -> pl.DataFrame:
    """Junta as planilhas no formato de mart.fuel_prices_anp_weekly.

    A semana da ANP vai de domingo a sábado: week_start vira a segunda-feira
    seguinte, como nas médias calculadas a partir dos postos.
    """
    frame = pl.concat(sheets)
    unknown = set(frame["product_anp"].unique()) - SERIES_PRODUCTS.keys() - IGNORED_PRODUCTS
    if unknown:
        raise SourceFormatError(f"produtos fora do catálogo na série da ANP: {sorted(unknown)}")

    frame = frame.filter(~pl.col("product_anp").is_in(IGNORED_PRODUCTS)).with_columns(
        product_id=pl.col("product_anp").replace_strict(SERIES_PRODUCTS),
        unit=pl.col("unit").str.replace("³", "3"),
    )
    wrong_unit = frame.filter(pl.col("unit") != pl.col("product_id").replace_strict(_UNITS))
    if not wrong_unit.is_empty():
        pairs = sorted(set(wrong_unit.select("product_anp", "unit").iter_rows()))
        raise SourceFormatError(f"unidades inesperadas na série da ANP: {pairs}")

    mart = frame.select(
        "product_id",
        week_start=pl.col("week_start_anp") + timedelta(days=1),
        avg_price="avg_price",
        n_stations="n_stations",
    ).sort("product_id", "week_start")
    return MartFuelPricesAnpWeekly.validate(mart)


def _within_history_range() -> pl.Expr:
    expr = pl.lit(False)
    for p in PRODUCTS:
        expr = (
            pl.when(pl.col("product_id") == p.id)
            .then(pl.col("avg_price").is_between(HISTORY_MIN_PRICE, p.max_price, closed="right"))
            .otherwise(expr)
        )
    return expr


class MartFuelPricesAnpWeekly(pa.DataFrameModel):
    """Contrato de mart.fuel_prices_anp_weekly."""

    product_id: pl.String = pa.Field(isin=PRODUCT_IDS)
    week_start: pl.Date
    avg_price: pl.Float64
    n_stations: pl.Int64 = pa.Field(gt=0)

    class Config:
        strict = True
        # A mesma semana em duas planilhas (ou repetida numa) é contradição
        # na fonte: falha em vez de escolher uma.
        unique = ["product_id", "week_start"]

    @pa.dataframe_check
    def price_within_history_range(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(_within_history_range())

    @pa.dataframe_check
    def week_starts_on_monday(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(pl.col("week_start").dt.weekday() == 1)


def compare_with_stations(
    official: pl.DataFrame, ours: pl.DataFrame, max_mean_diff_pct: float
) -> list[str]:
    """Avisos quando a média que o pipeline calcula a partir dos postos se
    afasta da média oficial da ANP nas semanas em comum.

    ours: product_id, week_start, avg_price (mart.fuel_prices_weekly, Brasil).
    Diferença média medida em 2026-11: 0,39% na gasolina (máxima 0,81%).
    """
    joined = ours.join(official, on=["product_id", "week_start"], suffix="_anp")
    stats = (
        joined.group_by("product_id")
        .agg(
            weeks=pl.len(),
            diff_pct=((pl.col("avg_price") / pl.col("avg_price_anp") - 1).abs() * 100).mean(),
        )
        .filter(pl.col("diff_pct") > max_mean_diff_pct)
        .sort("product_id")
    )
    return [
        f"{row['product_id']}: média dos postos difere {br_number(row['diff_pct'], 2)}% da média "
        f"oficial da ANP em {row['weeks']} semanas (limite {br_number(max_mean_diff_pct, 0)}%)"
        for row in stats.iter_rows(named=True)
    ]
