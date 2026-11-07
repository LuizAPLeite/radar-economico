"""Leitura do schema raw."""

from dataclasses import dataclass
from datetime import date

import polars as pl
import psycopg

from radar_pipeline.bcb.transform import RAW_SCHEMA

# Mesmo valor que o collector grava em raw.collection_runs.source.
SOURCE = "bcb_sgs"


@dataclass(frozen=True)
class Run:
    id: int
    as_of: date  # data da coleta no horário de Brasília


def latest_successful_run(conn: psycopg.Connection) -> Run | None:
    """Coleta bem-sucedida mais recente. Cada coleta traz o histórico
    completo, então ela basta para reconstruir staging e mart."""
    row = conn.execute(
        """
        SELECT id, (started_at AT TIME ZONE 'America/Sao_Paulo')::date
          FROM raw.collection_runs
         WHERE source = %s AND status = 'success'
         ORDER BY id DESC
         LIMIT 1
        """,
        (SOURCE,),
    ).fetchone()
    return Run(id=row[0], as_of=row[1]) if row else None


def read_responses(conn: psycopg.Connection, run_id: int) -> pl.DataFrame:
    """Respostas de uma coleta, com o payload como texto JSON."""
    rows = conn.execute(
        """
        SELECT series_code, http_status, payload::text
          FROM raw.bcb_sgs_responses
         WHERE run_id = %s
        """,
        (run_id,),
    ).fetchall()
    return pl.DataFrame(rows, schema=RAW_SCHEMA, orient="row")
