"""Job do Banco Central: lê do raw as coletas que ainda não processou, valida
e grava staging e mart."""

import logging
from dataclasses import dataclass
from datetime import date, datetime

import polars as pl
import psycopg

from radar_pipeline import runs
from radar_pipeline.bcb import extract, load, quality, transform

log = logging.getLogger(__name__)

# Nome do job no CLI e em mart.pipeline_runs.
NAME = "bcb"


@dataclass(frozen=True)
class Summary:
    source_run_id: int  # a coleta mais recente processada (raw.collection_runs.id)
    source_collected_at: datetime
    as_of: date
    runs: int  # quantas coletas foram processadas juntas
    staging_rows: int
    mart_rows: int  # valores que entraram ou mudaram na mart
    warnings: tuple[str, ...] = ()

    @property
    def rows_written(self) -> int:
        return self.mart_rows


def run(conn: psycopg.Connection, *, full_refresh: bool = False) -> Summary | None:
    """Processa as coletas novas e grava com upsert (nada é apagado).

    Com full_refresh, reprocessa todas as coletas do raw e reconstrói staging
    e mart do zero: serve para quando uma regra de transformação muda.
    """
    after = None if full_refresh else runs.last_source_run(conn, NAME)
    pending = extract.successful_runs(conn, after)
    if not pending:
        log.info("nenhuma coleta nova do BCB desde a %s; nada a processar", after)
        return None
    newest = pending[-1]

    raw = extract.read_responses(conn, [r.id for r in pending])
    observations = transform.explode_responses(raw)
    # As duas etapas abaixo validam com pandera: se o contrato for violado,
    # a exceção interrompe o job antes de qualquer gravação.
    staging = transform.to_staging(observations)
    mart = transform.to_mart_values(staging.frame, newest.as_of)

    log.info(
        "%d coleta(s) do BCB (%d a %d)%s: %d respostas, %d observações, %d datas futuras ignoradas",
        len(pending),
        pending[0].id,
        newest.id,
        ", carga completa" if full_refresh else "",
        raw.height,
        staging.frame.height,
        mart.future_dropped,
    )
    if staging.without_value:
        log.warning("%d observações sem valor foram descartadas", staging.without_value)
    if mart.unknown_series:
        log.warning("séries fora do catálogo (ficam só no staging): %s", mart.unknown_series)

    # Avisos não impedem a carga: vão para o log e para o registro da execução.
    warnings = quality.check_steps(mart.frame, extract.last_dates(conn))
    for warning in warnings:
        log.warning("aviso: %s", warning)

    if full_refresh:
        written = load.replace_all(conn, staging.frame, mart.frame, newest.id)
    else:
        written = load.upsert(conn, staging.frame, mart.frame, newest.id)

    for row in _latest_values(mart.frame).iter_rows(named=True):
        log.info("%s: último valor em %s = %s", *row.values())

    return Summary(
        source_run_id=newest.id,
        source_collected_at=newest.started_at,
        as_of=newest.as_of,
        runs=len(pending),
        staging_rows=staging.frame.height,
        mart_rows=written,
        warnings=tuple(warnings),
    )


def _latest_values(mart_values: pl.DataFrame) -> pl.DataFrame:
    return (
        mart_values.group_by("indicator_id")
        .agg(
            pl.col("ref_date").max().alias("last_date"),
            pl.col("value").sort_by("ref_date").last().alias("last_value"),
        )
        .sort("indicator_id")
    )
