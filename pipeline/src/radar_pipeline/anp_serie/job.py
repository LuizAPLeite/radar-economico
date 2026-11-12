"""Job da série histórica da ANP: lê as planilhas do raw, valida e grava
mart.fuel_prices_anp_weekly."""

import logging
from dataclasses import dataclass
from datetime import datetime

import polars as pl
import psycopg

from radar_pipeline import runs, source_files
from radar_pipeline.anp_serie import transform
from radar_pipeline.pg import copy_frame

log = logging.getLogger(__name__)

# Nome do job no CLI e em mart.pipeline_runs, e da fonte em raw.source_files.
NAME = "anp_serie"

# As duas planilhas precisam estar no raw: sem a antiga, a série começaria
# em 2013 sem ninguém perceber.
REQUIRED_FILES = ("semanal-brasil-2004-a-2012", "semanal-brasil-desde-2013")

# Diferença média aceitável entre a média calculada a partir dos postos e a
# oficial. Acima disso, provavelmente mudou algo na regra de um dos lados.
MAX_MEAN_DIFF_PCT = 2.0

# Produtos em que as duas médias são comparáveis. A média oficial não é a
# média simples dos postos (fica abaixo de qualquer média simples, mesmo com
# os mesmos postos; provavelmente pondera por volume vendido): onde o preço
# varia muito entre estados, a diferença é estrutural. Medido em 2026-11, em
# 48 semanas: gasolina 0,4%, aditivada 0,4%, diesel S10 0,5%, diesel 1,1%;
# etanol 5,2% (2% a 8%) e GNV 2,6%, que por isso ficam fora da comparação.
COMPARABLE_PRODUCTS = ("gasolina", "gasolina_aditivada", "diesel", "diesel_s10")


@dataclass(frozen=True)
class Summary:
    source_run_id: int
    source_collected_at: datetime
    weeks: int
    rows: int
    warnings: tuple[str, ...] = ()

    @property
    def rows_written(self) -> int:
        return self.rows


def run(conn: psycopg.Connection, *, full_refresh: bool = False) -> Summary | None:
    """Reconstrói a série oficial, se chegou planilha nova (ou com full_refresh)."""
    newest = source_files.newest_run(conn, NAME)
    if newest is None:
        log.warning("nenhuma planilha da série da ANP no raw; nada a processar")
        return None
    if not full_refresh and newest <= (runs.last_source_run(conn, NAME) or 0):
        log.info("nenhuma planilha nova da série da ANP desde a coleta %d", newest)
        return None

    files = source_files.latest(conn, NAME)
    missing = set(REQUIRED_FILES) - files.keys()
    if missing:
        raise RuntimeError(f"planilhas da série da ANP faltando no raw: {sorted(missing)}")

    mart = transform.to_mart([transform.parse_sheet(files[n].content) for n in REQUIRED_FILES])

    ours = pl.DataFrame(
        conn.execute(
            "SELECT product_id, week_start, avg_price::float8 "
            "FROM mart.fuel_prices_weekly WHERE scope = 'BR' AND product_id = ANY(%s)",
            (list(COMPARABLE_PRODUCTS),),
        ).fetchall(),
        schema={"product_id": pl.String, "week_start": pl.Date, "avg_price": pl.Float64},
        orient="row",
    )
    warnings = transform.compare_with_stations(mart, ours, MAX_MEAN_DIFF_PCT)
    for warning in warnings:
        log.warning("aviso: %s", warning)

    with conn.transaction(), conn.cursor() as cur:
        cur.execute("DELETE FROM mart.fuel_prices_anp_weekly")
        copy_frame(cur, ("mart", "fuel_prices_anp_weekly"), mart)
        cur.execute("ANALYZE mart.fuel_prices_anp_weekly")

    log.info(
        "série da ANP: %d linhas, %d semanas de %s a %s",
        mart.height,
        mart["week_start"].n_unique(),
        mart["week_start"].min(),
        mart["week_start"].max(),
    )
    used = [files[n] for n in REQUIRED_FILES]
    return Summary(
        source_run_id=max(f.run_id for f in used),
        source_collected_at=max(f.fetched_at for f in used),
        weeks=mart["week_start"].n_unique(),
        rows=mart.height,
        warnings=tuple(warnings),
    )
