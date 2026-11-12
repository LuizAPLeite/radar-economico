"""Registro das execuções (mart.pipeline_runs) num Postgres de verdade.

Roda numa transação que sempre sofre rollback; sem TEST_DATABASE_URL é pulado.
"""

import os
from dataclasses import dataclass
from datetime import UTC, datetime

import psycopg
import pytest

from radar_pipeline.analysis import job as analysis_job
from radar_pipeline.cli import run_job

pytestmark = pytest.mark.skipif(
    not os.environ.get("TEST_DATABASE_URL"), reason="TEST_DATABASE_URL não definida"
)

COLLECTED_AT = datetime(2026, 11, 7, 12, 0, tzinfo=UTC)


@pytest.fixture
def conn():
    with psycopg.connect(os.environ["TEST_DATABASE_URL"], autocommit=True) as conn:
        with conn.transaction(force_rollback=True):
            yield conn


@dataclass(frozen=True)
class FakeSummary:
    source_run_id: int = 42
    source_collected_at: datetime = COLLECTED_AT
    rows_written: int = 10
    warnings: tuple[str, ...] = ("semana de 26/10 sem dados do AC",)


def last_run(conn, job: str) -> dict:
    cur = conn.execute(
        """SELECT status, error, source_run_id, source_collected_at, rows_written, warnings,
                  finished_at >= started_at AS ordered
             FROM mart.pipeline_runs WHERE job = %s ORDER BY id DESC LIMIT 1""",
        (job,),
    )
    columns = [c.name for c in cur.description]
    return dict(zip(columns, cur.fetchone(), strict=True))


def test_success_is_recorded_with_the_summary(conn):
    assert run_job(conn, "teste-sucesso", lambda _conn: FakeSummary()) is True

    assert last_run(conn, "teste-sucesso") == {
        "status": "success",
        "error": None,
        "source_run_id": 42,
        "source_collected_at": COLLECTED_AT,
        "rows_written": 10,
        "warnings": ["semana de 26/10 sem dados do AC"],
        "ordered": True,
    }


def test_nothing_to_process_is_recorded_as_skipped(conn):
    assert run_job(conn, "teste-vazio", lambda _conn: None) is True

    run = last_run(conn, "teste-vazio")
    assert (run["status"], run["source_run_id"], run["rows_written"]) == ("skipped", None, None)


def test_failure_undoes_the_load_and_is_recorded(conn):
    before = conn.execute("SELECT count(*) FROM mart.fuel_products").fetchone()[0]

    def broken_job(conn):
        conn.execute("DELETE FROM mart.fuel_prices_municipal")
        conn.execute("DELETE FROM mart.fuel_prices_weekly")
        conn.execute("DELETE FROM mart.fuel_prices_anp_weekly")
        for table in analysis_job._TABLES:  # também referenciam os produtos
            conn.execute(f"DELETE FROM {table}")
        conn.execute("DELETE FROM mart.fuel_products")
        raise RuntimeError("arquivo da ANP com layout novo")

    assert run_job(conn, "teste-falha", broken_job) is False

    # O que o job apagou voltou (a transação dele foi desfeita)...
    assert conn.execute("SELECT count(*) FROM mart.fuel_products").fetchone()[0] == before
    # ...e a falha ficou registrada, com a mensagem.
    run = last_run(conn, "teste-falha")
    assert run["status"] == "failed"
    assert run["error"] == "RuntimeError: arquivo da ANP com layout novo"
