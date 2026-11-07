"""Ponto de entrada: `radar-pipeline` (ou `python -m radar_pipeline`).

Variáveis de ambiente:
    DATABASE_URL  obrigatória
"""

import logging
import os
import sys

import psycopg

from radar_pipeline.bcb import job as bcb_job

log = logging.getLogger("radar_pipeline")


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
        # autocommit: leituras não deixam transação aberta; a gravação abre
        # a sua própria transação explicitamente (ver load.replace_all).
        with psycopg.connect(database_url, autocommit=True) as conn:
            bcb_job.run(conn)
    except Exception:
        log.exception("pipeline falhou")
        sys.exit(1)
