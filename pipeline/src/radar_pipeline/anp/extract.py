"""Leitura dos arquivos da ANP no raw e dos municípios no staging."""

from dataclasses import dataclass
from datetime import date, datetime

import polars as pl
import psycopg


@dataclass(frozen=True)
class RawFile:
    id: int
    run_id: int  # coleta que gravou esta versão (raw.collection_runs.id)
    fetched_at: datetime
    product_group: str
    ref_month: date
    content_gzip: bytes


def newest_run(conn: psycopg.Connection) -> int | None:
    """A coleta mais recente que gravou algum arquivo (None se não há nenhum).

    Uma versão nova de um arquivo sempre vem de uma coleta mais nova que a da
    versão anterior, então, se este número não mudou, nenhum arquivo mudou.
    """
    return conn.execute("SELECT max(run_id) FROM raw.anp_files").fetchone()[0]


def latest_files(conn: psycopg.Connection) -> list[RawFile]:
    """A versão mais recente de cada arquivo (grupo e mês). O conteúdo vem
    compactado: o job descompacta um de cada vez, para não ter os ~130 MB
    de CSV na memória ao mesmo tempo."""
    rows = conn.execute(
        """
        SELECT DISTINCT ON (product_group, ref_month)
               id, run_id, fetched_at, product_group, ref_month, content_gzip
          FROM raw.anp_files
         ORDER BY product_group, ref_month, fetched_at DESC, id DESC
        """
    ).fetchall()
    return [RawFile(*row) for row in rows]


def last_national_weeks(conn: psycopg.Connection) -> dict[str, date]:
    """Semana mais recente de cada produto na mart (antes da carga nova)."""
    rows = conn.execute(
        """
        SELECT product_id, max(week_start)
          FROM mart.fuel_prices_weekly
         WHERE scope = 'BR'
         GROUP BY product_id
        """
    ).fetchall()
    return dict(rows)


def municipalities(conn: psycopg.Connection) -> pl.DataFrame:
    """Os municípios do IBGE (gravados pelo job do IBGE), para casar os nomes."""
    rows = conn.execute("SELECT ibge_code, name, uf FROM staging.municipalities").fetchall()
    return pl.DataFrame(
        rows, schema={"ibge_code": pl.Int64, "name": pl.String, "uf": pl.String}, orient="row"
    )
