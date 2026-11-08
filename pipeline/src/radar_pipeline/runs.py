"""Registro das execuções do pipeline em mart.pipeline_runs.

Cada job devolve um resumo (ou None quando não há nada no raw para
processar), e o CLI grava uma linha por execução, inclusive quando o job
falha. A API mostra esse registro no endpoint de saúde dos dados.
"""

from datetime import datetime
from typing import Any, Protocol

import psycopg
from psycopg.types.json import Jsonb

# Um erro do pandera pode trazer milhares de linhas com os casos que
# falharam; o registro guarda só o começo.
MAX_ERROR_CHARS = 2_000


class JobSummary(Protocol):
    """O que o resumo de todo job informa ao registro.

    Protocol é "tipagem por formato": qualquer classe com esses atributos
    serve, sem precisar herdar desta (os dataclasses dos jobs servem).
    """

    @property
    def source_run_id(self) -> int: ...  # coleta mais recente usada (raw.collection_runs.id)

    @property
    def source_collected_at(self) -> datetime: ...

    @property
    def rows_written(self) -> int: ...  # linhas inseridas ou atualizadas na mart

    @property
    def warnings(self) -> tuple[str, ...]: ...


# Parâmetros nomeados (%(nome)s): cada valor vai para a coluna certa sem
# depender da ordem de uma tupla.
_INSERT = """
INSERT INTO mart.pipeline_runs
    (job, started_at, finished_at, status, error,
     source_run_id, source_collected_at, rows_written, warnings)
VALUES
    (%(job)s, %(started_at)s, %(finished_at)s, %(status)s, %(error)s,
     %(source_run_id)s, %(source_collected_at)s, %(rows_written)s, %(warnings)s)
"""


def record(
    conn: psycopg.Connection,
    job: str,
    started_at: datetime,
    finished_at: datetime,
    summary: JobSummary | None,
) -> None:
    """Grava uma execução que terminou bem: success, ou skipped sem resumo."""
    params = _params(job, started_at, finished_at, status="skipped")
    if summary is not None:
        params |= {
            "status": "success",
            "source_run_id": summary.source_run_id,
            "source_collected_at": summary.source_collected_at,
            "rows_written": summary.rows_written,
            "warnings": Jsonb(list(summary.warnings)),
        }
    conn.execute(_INSERT, params)


def record_failure(
    conn: psycopg.Connection,
    job: str,
    started_at: datetime,
    finished_at: datetime,
    error: BaseException,
) -> None:
    params = _params(job, started_at, finished_at, status="failed")
    params["error"] = describe_error(error)
    conn.execute(_INSERT, params)


def _params(job: str, started_at: datetime, finished_at: datetime, status: str) -> dict[str, Any]:
    return {
        "job": job,
        "started_at": started_at,
        "finished_at": finished_at,
        "status": status,
        "error": None,
        "source_run_id": None,
        "source_collected_at": None,
        "rows_written": None,
        "warnings": Jsonb([]),
    }


def last_source_run(conn: psycopg.Connection, job: str) -> int | None:
    """A coleta mais recente que o job já processou com sucesso (None se nunca).

    É o ponto de partida das cargas incrementais: o job só precisa ler o que
    chegou ao raw depois dela.
    """
    return conn.execute(
        "SELECT max(source_run_id) FROM mart.pipeline_runs WHERE job = %s AND status = 'success'",
        (job,),
    ).fetchone()[0]


def describe_error(error: BaseException) -> str:
    """Tipo e mensagem do erro, cortados em MAX_ERROR_CHARS."""
    text = f"{type(error).__name__}: {error}"
    if len(text) <= MAX_ERROR_CHARS:
        return text
    return text[: MAX_ERROR_CHARS - 1] + "…"
