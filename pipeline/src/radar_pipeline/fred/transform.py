"""Transformações puras (sem banco) dos CSVs do FRED."""

import io
from dataclasses import dataclass

import pandera.polars as pa
import polars as pl
from pandera.polars import PolarsData

from radar_pipeline.anp.transform import SourceFormatError


@dataclass(frozen=True)
class Series:
    """Uma série do FRED publicada como indicador na mart.

    min_value (exclusivo) e max_value (inclusivo) são limites de sanidade,
    como no catálogo do BCB: pegam erro de escala, não são previsões.
    """

    id: str  # id do indicador na mart
    fred_id: str  # nome do arquivo em raw.source_files e da coluna no CSV
    name: str
    unit: str
    min_value: float
    max_value: float


# O Brent ficou entre US$ 9 (1998) e US$ 144 (2008) desde 1987.
BRENT = Series("brent", "DCOILBRENTEU", "Petróleo Brent", "US$/barril", 0, 500)
SERIES: tuple[Series, ...] = (BRENT,)


def parse_csv(content: bytes, series: Series) -> pl.DataFrame:
    """Lê o CSV de uma série e devolve indicator_id, ref_date e value.

    Formato (conferido em 2026-11-11): cabeçalho "observation_date,<id>",
    datas AAAA-MM-DD e valor com ponto decimal. Dias sem cotação (feriados
    nos EUA) vêm com o valor vazio (antes, com "."): ficam de fora.
    """
    frame = pl.read_csv(io.BytesIO(content), infer_schema=False)
    if frame.columns != ["observation_date", series.fred_id]:
        raise SourceFormatError(f"colunas inesperadas no CSV do FRED: {frame.columns}")
    values = (
        frame.rename({"observation_date": "ref_date", series.fred_id: "value"})
        .filter(pl.col("value").is_not_null() & (pl.col("value") != "."))
        .select(
            indicator_id=pl.lit(series.id),
            ref_date=pl.col("ref_date").str.to_date("%Y-%m-%d", strict=True),
            value=pl.col("value").cast(pl.Float64, strict=True),
        )
    )
    return IndicatorValues.validate(values)


def _within_series_range() -> pl.Expr:
    expr = pl.lit(False)  # série fora do catálogo nunca é válida
    for s in SERIES:
        expr = (
            pl.when(pl.col("indicator_id") == s.id)
            .then(pl.col("value").is_between(s.min_value, s.max_value, closed="right"))
            .otherwise(expr)
        )
    return expr


class IndicatorValues(pa.DataFrameModel):
    """Contrato dos valores de uma série do FRED antes de ir para a mart."""

    indicator_id: pl.String = pa.Field(isin=[s.id for s in SERIES])
    ref_date: pl.Date
    value: pl.Float64

    class Config:
        strict = True
        unique = ["indicator_id", "ref_date"]

    @pa.dataframe_check
    def value_within_series_range(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(_within_series_range())
