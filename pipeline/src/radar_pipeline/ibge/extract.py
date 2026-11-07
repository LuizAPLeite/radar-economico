"""Leitura das respostas do IBGE no raw."""

from typing import Any

import psycopg


def latest_payloads(conn: psycopg.Connection) -> dict[str, Any]:
    """A versão mais recente de cada conjunto ("municipios", "centroides").

    O collector só grava uma versão nova quando o conteúdo muda, então a mais
    recente é a atual, mesmo que tenha sido gravada por uma coleta antiga.
    O psycopg já devolve o jsonb como listas e dicionários do Python.
    """
    rows = conn.execute(
        """
        SELECT DISTINCT ON (dataset) dataset, payload
          FROM raw.ibge_responses
         ORDER BY dataset, fetched_at DESC, id DESC
        """
    ).fetchall()
    return dict(rows)
