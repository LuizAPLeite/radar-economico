"""Transformações puras (sem banco): raw -> staging -> mart.

Funções que recebem e devolvem DataFrames são fáceis de testar: os testes
montam a entrada na mão, sem Postgres e sem internet.
"""

from dataclasses import dataclass
from datetime import date

import polars as pl

from radar_pipeline.bcb.indicators import INDICATORS
from radar_pipeline.bcb.schemas import MartIndicatorValues, StagingObservations

# Colunas lidas de raw.bcb_sgs_responses (run_id é a coleta de cada resposta).
RAW_SCHEMA = {
    "run_id": pl.Int64,
    "series_code": pl.Int64,
    "http_status": pl.Int64,
    "payload": pl.String,
}

# Corpo das respostas 200 da API SGS: [{"data": "dd/MM/aaaa", "valor": "1.23"}, ...]
_SGS_PAYLOAD = pl.List(pl.Struct({"data": pl.String, "valor": pl.String}))


def explode_responses(raw: pl.DataFrame) -> pl.DataFrame:
    """Transforma cada resposta (um array JSON) em uma linha por observação.

    Respostas 404 ("período sem dados") não têm observações e são ignoradas.
    Os campos continuam como texto; a tipagem acontece em to_staging.
    """
    return (
        raw.filter(pl.col("http_status") == 200)
        .with_columns(obs=pl.col("payload").str.json_decode(_SGS_PAYLOAD))
        .explode("obs")
        .drop_nulls("obs")  # um array vazio vira uma linha nula no explode
        .unnest("obs")
        .select("run_id", "series_code", "data", "valor")
    )


@dataclass(frozen=True)
class StagingResult:
    frame: pl.DataFrame
    without_value: int  # observações publicadas sem valor (descartadas)


def to_staging(observations: pl.DataFrame) -> StagingResult:
    """Tipa, junta as coletas e valida as observações.

    - data "dd/MM/aaaa" -> date; valor "1.23" -> float. Texto fora do formato
      faz a conversão falhar (strict), em vez de virar nulo em silêncio.
    - Observações sem valor são descartadas e contadas.
    - Coletas diferentes se sobrepõem de propósito (a margem da coleta
      incremental): para cada série e data, vale a coleta mais recente, que
      traz o valor corrigido se o BCB tiver corrigido algum.
    - Datas repetidas dentro da MESMA coleta violam o contrato: as janelas de
      uma coleta não se sobrepõem, então isso indicaria um bug, e escolher
      uma das duas em silêncio esconderia o problema.
    """
    valor = pl.col("valor").str.strip_chars()
    with_value = observations.filter(valor.fill_null("") != "")
    frame = (
        with_value.select(
            "series_code",
            pl.col("data").str.to_date("%d/%m/%Y").alias("ref_date"),
            valor.cast(pl.Float64).alias("value"),
            "run_id",
        )
        .filter(pl.col("run_id") == pl.col("run_id").max().over("series_code", "ref_date"))
        .sort("series_code", "ref_date")
    )

    return StagingResult(
        frame=StagingObservations.validate(frame, lazy=True),
        without_value=observations.height - with_value.height,
    )


@dataclass(frozen=True)
class MartResult:
    frame: pl.DataFrame
    future_dropped: int  # datas depois de as_of (a meta Selic é projetada até o próximo Copom)
    unknown_series: list[int]  # séries do staging sem indicador no catálogo


def to_mart_values(staging: pl.DataFrame, as_of: date) -> MartResult:
    """Troca o código do SGS pelo id do indicador e descarta datas futuras."""
    catalog = pl.DataFrame(
        {
            "series_code": [i.sgs_code for i in INDICATORS],
            "indicator_id": [i.id for i in INDICATORS],
        },
        schema={"series_code": pl.Int64, "indicator_id": pl.String},
    )
    known = staging.join(catalog, on="series_code", how="inner")
    current = known.filter(pl.col("ref_date") <= as_of)
    frame = current.select("indicator_id", "ref_date", "value").sort("indicator_id", "ref_date")

    unknown = set(staging["series_code"].to_list()) - set(catalog["series_code"].to_list())
    return MartResult(
        frame=MartIndicatorValues.validate(frame, lazy=True),
        future_dropped=known.height - current.height,
        unknown_series=sorted(unknown),
    )
