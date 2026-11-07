"""Job do Banco Central: lê a coleta mais recente do raw, valida e grava
staging e mart."""

import logging
from dataclasses import dataclass
from datetime import date

import polars as pl
import psycopg

from radar_pipeline.bcb import extract, load, transform

log = logging.getLogger(__name__)


@dataclass(frozen=True)
class Summary:
    run_id: int
    as_of: date
    staging_rows: int
    mart_rows: int


def run(conn: psycopg.Connection) -> Summary | None:
    latest = extract.latest_successful_run(conn)
    if latest is None:
        log.warning("nenhuma coleta bem-sucedida em raw; nada a processar")
        return None

    raw = extract.read_responses(conn, latest.id)
    observations = transform.explode_responses(raw)
    # As duas etapas abaixo validam com pandera: se o contrato for violado,
    # a exceção interrompe o job antes de qualquer gravação.
    staging = transform.to_staging(observations, latest.id)
    mart = transform.to_mart_values(staging.frame, latest.as_of)

    log.info(
        "coleta %d (%s): %d respostas, %d observações, %d na mart, %d datas futuras ignoradas",
        latest.id,
        latest.as_of,
        raw.height,
        staging.frame.height,
        mart.frame.height,
        mart.future_dropped,
    )
    if staging.without_value:
        log.warning("%d observações sem valor foram descartadas", staging.without_value)
    if mart.unknown_series:
        log.warning("séries fora do catálogo (ficam só no staging): %s", mart.unknown_series)

    load.replace_all(conn, staging.frame, mart.frame, latest.id)

    for row in _latest_values(mart.frame).iter_rows(named=True):
        log.info(
            "%s: %d valores, último em %s = %s",
            row["indicator_id"],
            row["rows"],
            row["last_date"],
            row["last_value"],
        )

    return Summary(
        run_id=latest.id,
        as_of=latest.as_of,
        staging_rows=staging.frame.height,
        mart_rows=mart.frame.height,
    )


def _latest_values(mart_values: pl.DataFrame) -> pl.DataFrame:
    return (
        mart_values.group_by("indicator_id")
        .agg(
            pl.len().alias("rows"),
            pl.col("ref_date").max().alias("last_date"),
            pl.col("value").sort_by("ref_date").last().alias("last_value"),
        )
        .sort("indicator_id")
    )
