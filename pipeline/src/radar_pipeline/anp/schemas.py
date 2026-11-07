"""Contratos (pandera) dos preços de combustíveis."""

import pandera.polars as pa
import polars as pl
from pandera.polars import PolarsData

from radar_pipeline.anp.products import PRODUCT_IDS, PRODUCTS
from radar_pipeline.pg import UFS


def within_product_range(product: str, price: str) -> pl.Expr:
    """True quando o preço respeita os limites do seu combustível."""
    expr = pl.lit(False)  # produto fora do catálogo nunca é válido
    for p in PRODUCTS:
        expr = (
            pl.when(pl.col(product) == p.id)
            .then(pl.col(price).is_between(p.min_price, p.max_price, closed="right"))
            .otherwise(expr)
        )
    return expr


class StagingFuelPrices(pa.DataFrameModel):
    """Contrato de staging.fuel_prices: um preço por posto, produto e dia."""

    product: pl.String = pa.Field(isin=PRODUCT_IDS)
    collected_on: pl.Date
    cnpj: pl.String = pa.Field(str_matches=r"^\d{14}$")
    ibge_code: pl.Int64 = pa.Field(ge=1_000_000, le=9_999_999)
    uf: pl.String = pa.Field(isin=UFS)
    price: pl.Float64
    brand: pl.String
    source_file_id: pl.Int64 = pa.Field(gt=0)

    class Config:
        strict = True
        # O mesmo posto com dois preços diferentes no mesmo dia é uma
        # contradição na fonte: o pipeline falha em vez de escolher um.
        unique = ["product", "collected_on", "cnpj"]

    @pa.dataframe_check
    def price_within_product_range(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(within_product_range("product", "price"))


class MartFuelPricesWeekly(pa.DataFrameModel):
    """Contrato de mart.fuel_prices_weekly."""

    product_id: pl.String = pa.Field(isin=PRODUCT_IDS)
    scope: pl.String = pa.Field(isin=["BR", *UFS])
    week_start: pl.Date
    avg_price: pl.Float64
    n_stations: pl.Int64 = pa.Field(gt=0)

    class Config:
        strict = True
        unique = ["product_id", "scope", "week_start"]

    @pa.dataframe_check
    def avg_within_product_range(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(within_product_range("product_id", "avg_price"))

    @pa.dataframe_check
    def week_starts_on_monday(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(pl.col("week_start").dt.weekday() == 1)


class MartFuelPricesMunicipal(pa.DataFrameModel):
    """Contrato de mart.fuel_prices_municipal."""

    product_id: pl.String = pa.Field(isin=PRODUCT_IDS)
    month: pl.Date
    ibge_code: pl.Int64 = pa.Field(ge=1_000_000, le=9_999_999)
    avg_price: pl.Float64
    min_price: pl.Float64
    max_price: pl.Float64
    n_stations: pl.Int64 = pa.Field(gt=0)

    class Config:
        strict = True
        unique = ["product_id", "month", "ibge_code"]

    @pa.dataframe_check
    def avg_within_product_range(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(within_product_range("product_id", "avg_price"))

    @pa.dataframe_check
    def min_avg_max_in_order(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(
            (pl.col("min_price") <= pl.col("avg_price"))
            & (pl.col("avg_price") <= pl.col("max_price"))
        )

    @pa.dataframe_check
    def month_is_first_day(cls, data: PolarsData) -> pl.LazyFrame:
        return data.lazyframe.select(pl.col("month").dt.day() == 1)
