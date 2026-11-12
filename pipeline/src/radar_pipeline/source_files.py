"""Leitura dos arquivos inteiros guardados em raw.source_files (planilhas da
série histórica da ANP, CSV do FRED)."""

import gzip
from dataclasses import dataclass
from datetime import datetime

import psycopg


@dataclass(frozen=True)
class SourceFile:
    name: str
    run_id: int  # coleta que gravou esta versão (raw.collection_runs.id)
    fetched_at: datetime
    content: bytes  # já descompactado


def newest_run(conn: psycopg.Connection, source: str) -> int | None:
    """A coleta mais recente que gravou algum arquivo da fonte (None se nenhuma).

    O coletor só grava quando o conteúdo muda: se este número não passou do
    último processado, nada mudou na fonte.
    """
    return conn.execute(
        "SELECT max(run_id) FROM raw.source_files WHERE source = %s", (source,)
    ).fetchone()[0]


def latest(conn: psycopg.Connection, source: str) -> dict[str, SourceFile]:
    """A versão mais recente de cada arquivo da fonte, por nome."""
    rows = conn.execute(
        """
        SELECT DISTINCT ON (name) name, run_id, fetched_at, content_gzip
          FROM raw.source_files
         WHERE source = %s
         ORDER BY name, fetched_at DESC, id DESC
        """,
        (source,),
    ).fetchall()
    return {
        name: SourceFile(name, run_id, fetched_at, gzip.decompress(content))
        for name, run_id, fetched_at, content in rows
    }
