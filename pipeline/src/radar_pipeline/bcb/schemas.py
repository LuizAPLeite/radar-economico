"""Contratos de dados (pandera) de cada camada.

Validar ANTES de gravar garante que dado inválido nunca chega à mart:
se algo quebrar o contrato, o pipeline falha e a mart fica como estava.
"""

import pandera.polars as pa
import polars as pl
from pandera.polars import PolarsData

from radar_pipeline.bcb.indicators import INDICATORS


def within_indicator_range() -> pl.Expr:
    """Expressão que é True quando o valor respeita os limites do seu indicador."""
    expr = pl.lit(False)  # indicador fora do catálogo nunca é válido
    for ind in INDICATORS:
        expr = (
            pl.when(pl.col("indicator_id") == ind.id)
            .then(pl.col("value").is_between(ind.min_value, ind.max_value, closed="right"))
            .otherwise(expr)
        )
    return expr


class StagingObservations(pa.DataFrameModel):
    """Contrato de staging.bcb_sgs_observations."""

    series_code: pl.Int64 = pa.Field(gt=0)
    ref_date: pl.Date
    value: pl.Float64  # colunas não são anuláveis por padrão
    run_id: pl.Int64 = pa.Field(gt=0)

    class Config:
        strict = True  # rejeita colunas fora do contrato
        unique = ["series_code", "ref_date"]


class MartIndicatorValues(pa.DataFrameModel):
    """Contrato de mart.indicator_values."""

    indicator_id: pl.String = pa.Field(isin=[i.id for i in INDICATORS])
    ref_date: pl.Date
    value: pl.Float64

    class Config:
        strict = True
        unique = ["indicator_id", "ref_date"]

    @pa.dataframe_check
    def value_within_indicator_range(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(within_indicator_range())
