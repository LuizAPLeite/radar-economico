"""Ponto de entrada: `radar-pipeline` (ou `python -m radar_pipeline`).

Variáveis de ambiente:
    DATABASE_URL           obrigatória
    PIPELINE_JOBS          jobs a rodar, separados por vírgula (padrão: todos,
                           na ordem bcb, fred, ibge, anp, anp_serie)
    PIPELINE_FULL_REFRESH  true: reprocessa todo o raw e reconstrói staging e
                           mart, mesmo sem nada novo (padrão: só o que chegou)
"""

import functools
import logging
import os
import sys
from collections.abc import Callable
from datetime import UTC, datetime

import psycopg

from radar_pipeline import runs
from radar_pipeline.anp import job as anp_job
from radar_pipeline.anp_serie import job as anp_serie_job
from radar_pipeline.bcb import job as bcb_job
from radar_pipeline.fred import job as fred_job
from radar_pipeline.ibge import job as ibge_job

log = logging.getLogger("radar_pipeline")

Job = Callable[[psycopg.Connection], runs.JobSummary | None]

# A ordem importa: o job da ANP casa os municípios com a tabela que o job do
# IBGE grava, e o da série da ANP compara a média oficial com a que o job da
# ANP calcula. Os jobs não se chamam: cada um lê o que precisa do banco.
JOBS: dict[str, Job] = {
    bcb_job.NAME: bcb_job.run,
    fred_job.NAME: fred_job.run,
    ibge_job.NAME: ibge_job.run,
    anp_job.NAME: anp_job.run,
    anp_serie_job.NAME: anp_serie_job.run,
}


def parse_flag(name: str, value: str | None) -> bool:
    """Lê uma variável booleana: vazia, "false" ou "0" desliga; "true" ou "1" liga."""
    normalized = (value or "").strip().lower()
    if normalized in ("", "0", "false"):
        return False
    if normalized in ("1", "true"):
        return True
    raise ValueError(f"{name}: valor inválido {value!r} (use true ou false)")


def select_jobs(value: str | None) -> list[str]:
    """Os jobs pedidos em PIPELINE_JOBS, na ordem de JOBS (vazio = todos)."""
    if not value or not value.strip():
        return list(JOBS)
    requested = {name.strip() for name in value.split(",") if name.strip()}
    unknown = requested - JOBS.keys()
    if unknown:
        raise ValueError(
            f"PIPELINE_JOBS: jobs desconhecidos {sorted(unknown)} (válidos: {list(JOBS)})"
        )
    return [name for name in JOBS if name in requested]


def run_job(conn: psycopg.Connection, name: str, job: Job) -> bool:
    """Roda um job e grava a execução em mart.pipeline_runs.

    A carga do job e o registro do sucesso vão na mesma transação (a do job
    vira um SAVEPOINT dentro desta): ou os dois ficam, ou nenhum. Se o job
    falhar, tudo o que ele gravou é desfeito, e a falha é registrada depois,
    numa transação própria. Devolve True se o job terminou bem.
    """
    started_at = datetime.now(UTC)
    try:
        with conn.transaction():
            summary = job(conn)
            runs.record(conn, name, started_at, datetime.now(UTC), summary)
    except Exception as error:
        log.exception("job %s falhou", name)
        try:
            runs.record_failure(conn, name, started_at, datetime.now(UTC), error)
        except Exception:
            # Sem conexão com o banco, nem o registro da falha é possível.
            log.exception("não foi possível registrar a falha do job %s", name)
        return False

    seconds = (datetime.now(UTC) - started_at).total_seconds()
    if summary is None:
        log.info("job %s: nada a processar (%.1f s)", name, seconds)
    else:
        log.info(
            "job %s: %d linhas gravadas na mart, coleta %d (%.1f s)",
            name,
            summary.rows_written,
            summary.source_run_id,
            seconds,
        )
    return True


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )
    database_url = os.environ.get("DATABASE_URL")
    if not database_url:
        log.error("DATABASE_URL não definida")
        sys.exit(1)
    try:
        names = select_jobs(os.environ.get("PIPELINE_JOBS"))
        full_refresh = parse_flag("PIPELINE_FULL_REFRESH", os.environ.get("PIPELINE_FULL_REFRESH"))
    except ValueError as e:
        log.error("%s", e)
        sys.exit(1)
    if full_refresh:
        log.info("carga completa: todo o raw será reprocessado")

    failed = []
    # autocommit: leituras fora de um job não deixam transação aberta; cada
    # job roda dentro da sua própria transação (ver run_job).
    with psycopg.connect(database_url, autocommit=True) as conn:
        for name in names:
            # partial "fixa" um argumento: o resultado é uma função que só
            # recebe a conexão, como run_job espera.
            job = functools.partial(JOBS[name], full_refresh=full_refresh)
            # Um job que falha não impede os outros: a mart dele fica como
            # estava e as demais seguem atualizadas.
            if not run_job(conn, name, job):
                failed.append(name)

    if failed:
        log.error("jobs com falha: %s", failed)
        sys.exit(1)
