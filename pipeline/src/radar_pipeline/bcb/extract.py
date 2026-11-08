"""Leitura do schema raw."""

from dataclasses import dataclass
from datetime import date, datetime

import polars as pl
import psycopg

from radar_pipeline.bcb.transform import RAW_SCHEMA

# Mesmo valor que o collector grava em raw.collection_runs.source.
SOURCE = "bcb_sgs"


@dataclass(frozen=True)
class Run:
    id: int
    started_at: datetime
    as_of: date  # data da coleta no horário de Brasília


def successful_runs(conn: psycopg.Connection, after: int | None) -> list[Run]:
    """Coletas bem-sucedidas com id maior que `after` (None = todas), em ordem.

    A coleta é incremental (cada uma traz só as semanas recentes), então o
    job junta todas as que ainda não processou. Coletas que falharam ficam
    de fora: podem ter parado no meio.
    """
    rows = conn.execute(
        """
        SELECT id, started_at, (started_at AT TIME ZONE 'America/Sao_Paulo')::date
          FROM raw.collection_runs
         WHERE source = %s AND status = 'success' AND id > %s
         ORDER BY id
        """,
        (SOURCE, after or 0),
    ).fetchall()
    return [Run(*row) for row in rows]


def last_dates(conn: psycopg.Connection) -> dict[str, date]:
    """Data do último valor de cada indicador na mart (antes da carga nova)."""
    rows = conn.execute(
        "SELECT indicator_id, max(ref_date) FROM mart.indicator_values GROUP BY indicator_id"
    ).fetchall()
    return dict(rows)


def read_responses(conn: psycopg.Connection, run_ids: list[int]) -> pl.DataFrame:
    """Respostas das coletas dadas, com o payload como texto JSON."""
    rows = conn.execute(
        """
        SELECT run_id, series_code, http_status, payload::text
          FROM raw.bcb_sgs_responses
         WHERE run_id = ANY(%s)
        """,
        (run_ids,),
    ).fetchall()
    return pl.DataFrame(rows, schema=RAW_SCHEMA, orient="row")
