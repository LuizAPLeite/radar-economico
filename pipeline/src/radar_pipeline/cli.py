"""Ponto de entrada: `radar-pipeline` (ou `python -m radar_pipeline`).

Variáveis de ambiente:
    DATABASE_URL   obrigatória
    PIPELINE_JOBS  jobs a rodar, separados por vírgula (padrão: todos, na
                   ordem bcb, ibge, anp)
"""

import logging
import os
import sys
from collections.abc import Callable

import psycopg

from radar_pipeline.anp import job as anp_job
from radar_pipeline.bcb import job as bcb_job
from radar_pipeline.ibge import job as ibge_job

log = logging.getLogger("radar_pipeline")

# A ordem importa: o job da ANP casa os municípios com a tabela que o job do
# IBGE grava. Os jobs não se chamam: cada um lê o que precisa do banco.
JOBS: dict[str, Callable[[psycopg.Connection], object]] = {
    "bcb": bcb_job.run,
    "ibge": ibge_job.run,
    "anp": anp_job.run,
}


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
    except ValueError as e:
        log.error("%s", e)
        sys.exit(1)

    failed = []
    # autocommit: leituras não deixam transação aberta; cada gravação abre a
    # sua própria transação explicitamente (ver os load.py).
    with psycopg.connect(database_url, autocommit=True) as conn:
        for name in names:
            # Um job que falha não impede os outros: a mart dele fica como
            # estava (a transação é desfeita) e as demais seguem atualizadas.
            try:
                JOBS[name](conn)
            except Exception:
                log.exception("job %s falhou", name)
                failed.append(name)

    if failed:
        log.error("jobs com falha: %s", failed)
        sys.exit(1)
