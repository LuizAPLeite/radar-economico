"""Job da ANP: lê os CSVs do raw, valida, casa os municípios com o IBGE e
grava staging e mart."""

import gzip
import logging
from dataclasses import dataclass
from datetime import datetime

import polars as pl
import psycopg

from radar_pipeline import runs
from radar_pipeline.anp import extract, load, quality, transform

log = logging.getLogger(__name__)

# Nome do job no CLI e em mart.pipeline_runs.
NAME = "anp"


@dataclass(frozen=True)
class Summary:
    # Os arquivos podem vir de coletas diferentes (cada coleta só grava os
    # meses que mudaram): vale a mais recente entre eles.
    source_run_id: int
    source_collected_at: datetime
    files: int
    staging_rows: int
    duplicates_removed: int
    weekly_rows: int
    municipal_rows: int
    warnings: tuple[str, ...] = ()

    @property
    def rows_written(self) -> int:
        return self.weekly_rows + self.municipal_rows


def run(conn: psycopg.Connection, *, full_refresh: bool = False) -> Summary | None:
    """Reconstrói staging e mart dos combustíveis, se chegou arquivo novo.

    Sem arquivo novo desde a última carga, termina sem fazer nada (o
    coletor só grava um arquivo quando o conteúdo muda). Quando chega, a
    reconstrução inteira leva poucos segundos: reprocessar só o mês que mudou
    não compensaria a complexidade. full_refresh reconstrói mesmo sem novidade
    (útil quando uma regra de transformação muda).
    """
    newest = extract.newest_run(conn)
    if newest is None:
        log.warning("nenhum arquivo da ANP no raw; nada a processar")
        return None
    if not full_refresh and newest <= (runs.last_source_run(conn, NAME) or 0):
        log.info("nenhum arquivo novo da ANP desde a coleta %d; nada a processar", newest)
        return None

    files = extract.latest_files(conn)

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

    # Avisos não impedem a carga: vão para o log e para o registro da execução.
    warnings = quality.check_month_volumes(staging.frame) + quality.check_new_weeks(
        weekly, extract.last_national_weeks(conn)
    )
    for warning in warnings:
        log.warning("aviso: %s", warning)

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
        source_run_id=max(f.run_id for f in files),
        source_collected_at=max(f.fetched_at for f in files),
        files=len(files),
        staging_rows=staging.frame.height,
        duplicates_removed=staging.duplicates_removed,
        weekly_rows=weekly.height,
        municipal_rows=municipal.height,
        warnings=tuple(warnings),
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
