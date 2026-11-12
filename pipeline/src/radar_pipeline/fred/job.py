"""Job do FRED: lê os CSVs do raw, valida e grava as séries como indicadores
da mart (mart.indicators e mart.indicator_values, ao lado dos do BCB)."""

import logging
from dataclasses import dataclass
from datetime import datetime

import polars as pl
import psycopg

from radar_pipeline import runs, source_files
from radar_pipeline.fred import transform
from radar_pipeline.pg import copy_frame

log = logging.getLogger(__name__)

# Nome do job no CLI e em mart.pipeline_runs, e da fonte em raw.source_files.
NAME = "fred"

_UPSERT_INDICATOR = """
INSERT INTO mart.indicators
    (id, name, unit, frequency, source, source_code, source_run_id, updated_at)
VALUES (%s, %s, %s, 'daily', 'FRED', %s, %s, now())
ON CONFLICT (id) DO UPDATE SET
    name          = EXCLUDED.name,
    unit          = EXCLUDED.unit,
    frequency     = EXCLUDED.frequency,
    source        = EXCLUDED.source,
    source_code   = EXCLUDED.source_code,
    source_run_id = EXCLUDED.source_run_id,
    updated_at    = EXCLUDED.updated_at
"""


@dataclass(frozen=True)
class Summary:
    source_run_id: int
    source_collected_at: datetime
    values: int
    warnings: tuple[str, ...] = ()

    @property
    def rows_written(self) -> int:
        return self.values


def run(conn: psycopg.Connection, *, full_refresh: bool = False) -> Summary | None:
    """Reconstrói as séries do FRED, se chegou CSV novo (ou com full_refresh).

    O CSV traz a série inteira e o FRED às vezes revisa valores antigos: a
    série é substituída por completo (são ~10 mil linhas, uma fração de
    segundo), em vez de só somar as datas novas.
    """
    newest = source_files.newest_run(conn, NAME)
    if newest is None:
        log.warning("nenhum CSV do FRED no raw; nada a processar")
        return None
    if not full_refresh and newest <= (runs.last_source_run(conn, NAME) or 0):
        log.info("nenhum CSV novo do FRED desde a coleta %d", newest)
        return None

    files = source_files.latest(conn, NAME)
    missing = {s.fred_id for s in transform.SERIES} - files.keys()
    if missing:
        raise RuntimeError(f"séries do FRED faltando no raw: {sorted(missing)}")

    used = [files[s.fred_id] for s in transform.SERIES]
    values = pl.concat(transform.parse_csv(files[s.fred_id].content, s) for s in transform.SERIES)

    with conn.transaction(), conn.cursor() as cur:
        cur.executemany(
            _UPSERT_INDICATOR,
            [(s.id, s.name, s.unit, s.fred_id, files[s.fred_id].run_id) for s in transform.SERIES],
        )
        cur.execute(
            "DELETE FROM mart.indicator_values WHERE indicator_id = ANY(%s)",
            ([s.id for s in transform.SERIES],),
        )
        copy_frame(cur, ("mart", "indicator_values"), values)
        cur.execute("ANALYZE mart.indicator_values")

    for s in transform.SERIES:
        last = values.filter(pl.col("indicator_id") == s.id).sort("ref_date").row(-1, named=True)
        log.info("%s: %s em %s", s.name, last["value"], last["ref_date"])

    return Summary(
        source_run_id=max(f.run_id for f in used),
        source_collected_at=max(f.fetched_at for f in used),
        values=values.height,
    )
