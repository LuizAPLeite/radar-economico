"""Job da ANP: lê os CSVs do raw, valida, casa os municípios com o IBGE e
grava staging e mart."""

import gzip
import logging
from dataclasses import dataclass

import polars as pl
import psycopg

from radar_pipeline.anp import extract, load, transform

log = logging.getLogger(__name__)


@dataclass(frozen=True)
class Summary:
    files: int
    staging_rows: int
    duplicates_removed: int
    weekly_rows: int
    municipal_rows: int


def run(conn: psycopg.Connection) -> Summary | None:
    files = extract.latest_files(conn)
    if not files:
        log.warning("nenhum arquivo da ANP no raw; nada a processar")
        return None

    municipalities = extract.municipalities(conn)
    if municipalities.is_empty():
        raise RuntimeError("staging.municipalities está vazia: o job do IBGE precisa rodar antes")

    # Descompacta e lê um arquivo de cada vez: só os DataFrames (bem menores
    # que o texto do CSV) ficam na memória.
    frames = [transform.parse_file(gzip.decompress(f.content_gzip), f.id) for f in files]
    staging = transform.to_staging(frames, municipalities)
    weekly_result = transform.weekly_averages(staging.frame)
    weekly = weekly_result.frame
    municipal = transform.municipal_monthly(staging.frame)

    first, last = staging.frame["collected_on"].min(), staging.frame["collected_on"].max()
    log.info(
        "anp: %d arquivos, %d preços de %s a %s (%d cópias idênticas removidas), "
        "%d médias semanais (%d semanas incompletas fora), %d médias mensais por município",
        len(files),
        staging.frame.height,
        first,
        last,
        staging.duplicates_removed,
        weekly.height,
        weekly_result.incomplete_weeks,
        municipal.height,
    )

    load.replace_all(conn, staging.frame, weekly, municipal)

    for row in _latest_national(weekly).iter_rows(named=True):
        log.info(
            "%s: R$ %.3f na semana de %s (%d postos)",
            row["product_id"],
            row["avg_price"],
            row["week_start"],
            row["n_stations"],
        )

    return Summary(
        files=len(files),
        staging_rows=staging.frame.height,
        duplicates_removed=staging.duplicates_removed,
        weekly_rows=weekly.height,
        municipal_rows=municipal.height,
    )


def _latest_national(weekly: pl.DataFrame) -> pl.DataFrame:
    """A média nacional da semana mais recente de cada produto."""
    return (
        weekly.filter(pl.col("scope") == "BR")
        .sort("week_start")
        .group_by("product_id", maintain_order=True)
        .last()
        .sort("product_id")
    )
