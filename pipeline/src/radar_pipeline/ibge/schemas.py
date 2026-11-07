"""Contrato (pandera) dos municípios."""

import pandera.polars as pa
import polars as pl

from radar_pipeline.pg import UFS


class StagingMunicipalities(pa.DataFrameModel):
    """Contrato de staging.municipalities."""

    ibge_code: pl.Int64 = pa.Field(ge=1_000_000, le=9_999_999)  # 7 dígitos
    name: pl.String = pa.Field(str_length={"min_value": 1})
    uf: pl.String = pa.Field(isin=UFS)
    # Limites do território brasileiro (com folga): pegam latitude e
    # longitude trocadas, ou coordenadas em outra escala.
    latitude: pl.Float64 = pa.Field(nullable=True, in_range={"min_value": -34.0, "max_value": 6.0})
    longitude: pl.Float64 = pa.Field(
        nullable=True, in_range={"min_value": -75.0, "max_value": -28.0}
    )

    class Config:
        strict = True
        unique = ["ibge_code"]
