"""Integração do job de análise num Postgres de verdade (transação desfeita no fim)."""

import os
from datetime import UTC, date, datetime, timedelta

import numpy as np
import psycopg
import pytest

from radar_pipeline.analysis import job
from radar_pipeline.cli import run_job

pytestmark = pytest.mark.skipif(
    not os.environ.get("TEST_DATABASE_URL"), reason="TEST_DATABASE_URL não definida"
)

START = date(2020, 1, 6)  # segunda-feira
N_WEEKS = 300


@pytest.fixture
def conn():
    with psycopg.connect(os.environ["TEST_DATABASE_URL"], autocommit=True) as conn:
        with conn.transaction(force_rollback=True):
            for table in (*job._TABLES, "mart.fuel_prices_anp_weekly", "mart.indicator_values"):
                conn.execute(f"DELETE FROM {table}")
            conn.execute("DELETE FROM mart.pipeline_runs")
            yield conn


def seed(conn) -> int:
    """Gasolina semanal e dólar/Brent diários sintéticos, e uma carga de
    origem bem-sucedida em pipeline_runs. Devolve o id da coleta."""
    rng = np.random.default_rng(1)
    run_id = conn.execute(
        "INSERT INTO raw.collection_runs (source, status) VALUES ('teste', 'success') RETURNING id"
    ).fetchone()[0]
    # O banco do CI começa vazio: o teste não pode contar com o que um job já gravou.
    conn.execute(
        """INSERT INTO mart.fuel_products (id, name, unit)
           VALUES ('gasolina', 'Gasolina comum', 'R$/l')
           ON CONFLICT (id) DO NOTHING"""
    )
    for indicator, source in (("usd_brl", "BCB"), ("brent", "FRED")):
        conn.execute(
            """INSERT INTO mart.indicators
                   (id, name, unit, frequency, source, source_code, source_run_id)
               VALUES (%s, %s, 'x', 'daily', %s, 'x', %s)
               ON CONFLICT (id) DO NOTHING""",
            (indicator, indicator, source, run_id),
        )
    days = [START + timedelta(days=d) for d in range(N_WEEKS * 7)]
    usd = 5 * np.exp(np.cumsum(rng.normal(0, 0.003, len(days))))
    brent = 80 * np.exp(np.cumsum(rng.normal(0, 0.01, len(days))))
    with conn.cursor() as cur:
        cur.executemany(
            "INSERT INTO mart.indicator_values VALUES (%s, %s, %s)",
            [("usd_brl", d, float(v)) for d, v in zip(days, usd, strict=True)]
            + [("brent", d, float(v)) for d, v in zip(days, brent, strict=True)],
        )
        cur.executemany(
            "INSERT INTO mart.fuel_prices_anp_weekly VALUES ('gasolina', %s, %s, 1000)",
            [(START + timedelta(weeks=w), float(6 * usd[w * 7] / 5)) for w in range(N_WEEKS)],
        )
    now = datetime.now(UTC)
    conn.execute(
        """INSERT INTO mart.pipeline_runs
               (job, started_at, finished_at, status, source_run_id, source_collected_at)
           VALUES ('anp_serie', %s, %s, 'success', %s, %s)""",
        (now, now, run_id, now),
    )
    return run_id


def count(conn, table: str) -> int:
    return conn.execute(f"SELECT count(*) FROM {table}").fetchone()[0]


def test_job_writes_results_and_skips_without_new_data(conn):
    run_id = seed(conn)

    assert run_job(conn, job.NAME, job.run)

    last = conn.execute(
        "SELECT status, source_run_id FROM mart.pipeline_runs WHERE job = 'analysis'"
    ).fetchone()
    assert last == ("success", run_id)
    assert count(conn, "mart.analysis_regimes") == 3
    assert count(conn, "mart.analysis_lag_correlations") > 0
    assert count(conn, "mart.analysis_rolling_correlations") > 0
    assert count(conn, "mart.fuel_forecasts") == 8
    assert count(conn, "mart.forecast_backtests") == 16
    # Só a gasolina tem série no teste: os outros produtos viram aviso.
    warnings = conn.execute(
        "SELECT warnings FROM mart.pipeline_runs WHERE job = 'analysis'"
    ).fetchone()[0]
    assert len(warnings) == 3

    assert run_job(conn, job.NAME, job.run)
    statuses = conn.execute(
        "SELECT status FROM mart.pipeline_runs WHERE job = 'analysis' ORDER BY id"
    ).fetchall()
    assert statuses == [("success",), ("skipped",)]


def test_without_source_jobs_there_is_nothing_to_do(conn):
    assert job.run(conn) is None
