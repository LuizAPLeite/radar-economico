"""Leitura das respostas do IBGE no raw."""

from dataclasses import dataclass
from datetime import datetime
from typing import Any

import psycopg


@dataclass(frozen=True)
class Payload:
    run_id: int  # coleta que gravou esta versão (raw.collection_runs.id)
    fetched_at: datetime
    data: Any  # o JSON da resposta, já como listas e dicionários do Python


def newest_run(conn: psycopg.Connection) -> int | None:
    """A coleta mais recente que gravou alguma resposta (None se não há nenhuma).

    O coletor só grava quando o conteúdo muda: se este número não mudou,
    nada mudou no IBGE.
    """
    return conn.execute("SELECT max(run_id) FROM raw.ibge_responses").fetchone()[0]


def latest_payloads(conn: psycopg.Connection) -> dict[str, Payload]:
    """A versão mais recente de cada conjunto ("municipios", "centroides").

    O collector só grava uma versão nova quando o conteúdo muda, então a mais
    recente é a atual, mesmo que tenha sido gravada por uma coleta antiga.
    O psycopg já devolve o jsonb como listas e dicionários do Python.
    """
    rows = conn.execute(
        """
        SELECT DISTINCT ON (dataset) dataset, run_id, fetched_at, payload
          FROM raw.ibge_responses
         ORDER BY dataset, fetched_at DESC, id DESC
        """
    ).fetchall()
    return {
        dataset: Payload(run_id, fetched_at, data) for dataset, run_id, fetched_at, data in rows
    }
