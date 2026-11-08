"""Teste de integração do job: raw -> staging -> mart num Postgres de verdade.

Precisa de um banco com as migrations aplicadas; sem TEST_DATABASE_URL é pulado.

    TEST_DATABASE_URL=postgres://radar:radar@localhost:5432/radar?sslmode=disable uv run pytest
"""

import json
import os
from datetime import UTC, date, datetime, timedelta, timezone

import psycopg
import pytest
from pandera.errors import SchemaErrors

from radar_pipeline.bcb import job
from radar_pipeline.cli import run_job

pytestmark = pytest.mark.skipif(
    not os.environ.get("TEST_DATABASE_URL"), reason="TEST_DATABASE_URL não definida"
)

BRT = timezone(timedelta(hours=-3))


@pytest.fixture
def conn():
    """Conexão dentro de uma transação que SEMPRE sofre rollback no fim.

    A transação do job vira um SAVEPOINT dentro desta: o teste enxerga tudo
    o que o job gravou e, no final, o banco volta exatamente ao que era.
    Assim o teste pode rodar até contra o banco de desenvolvimento.
    """
    with psycopg.connect(os.environ["TEST_DATABASE_URL"], autocommit=True) as conn:
        with conn.transaction(force_rollback=True):
            yield conn


def insert_run(conn, started_at: datetime, responses: list[tuple[int, int, object]]) -> int:
    run_id = conn.execute(
        """INSERT INTO raw.collection_runs (source, started_at, finished_at, status)
           VALUES ('bcb_sgs', %s, %s, 'success') RETURNING id""",
        (started_at, started_at),
    ).fetchone()[0]
    for code, status, payload in responses:
        conn.execute(
            """INSERT INTO raw.bcb_sgs_responses
                   (run_id, series_code, request_url, http_status, payload)
               VALUES (%s, %s, 'http://teste', %s, %s::jsonb)""",
            (run_id, code, status, json.dumps(payload)),
        )
    return run_id


def obs(data: str, valor: str) -> dict[str, str]:
    return {"data": data, "valor": valor}


# 23:30 em Brasília já é dia 08 em UTC: o "hoje" da coleta tem que ser 07/11.
LATE_NIGHT = datetime(2026, 11, 7, 23, 30, tzinfo=BRT)

VALID_RESPONSES = [
    (1, 200, [obs("05/11/2026", "5.4621"), obs("06/11/2026", "5.5013")]),
    (1, 404, {"erro": {"statusCode": 404, "detail": "Value(s) not found"}}),
    (433, 200, [obs("01/08/2026", "-0.32"), obs("01/09/2026", "0.82")]),
    (432, 200, [obs("07/11/2026", "13.75"), obs("08/11/2026", "13.75")]),  # 08/11 é futuro
    (9999, 200, [obs("06/11/2026", "1.0")]),  # série fora do catálogo
]


def count(conn, sql: str, *params) -> int:
    return conn.execute(sql, params).fetchone()[0]


def test_job_fills_staging_and_mart(conn):
    run_id = insert_run(conn, LATE_NIGHT, VALID_RESPONSES)

    summary = job.run(conn)

    assert summary is not None
    assert summary.source_run_id == run_id
    assert summary.source_collected_at == LATE_NIGHT
    assert summary.as_of == date(2026, 11, 7)

    # staging tem tudo, inclusive a data futura e a série fora do catálogo
    assert count(conn, "SELECT count(*) FROM staging.bcb_sgs_observations") == 7
    assert (
        count(conn, "SELECT count(*) FROM staging.bcb_sgs_observations WHERE run_id = %s", run_id)
        == 7
    )

    # mart: só indicadores conhecidos e sem a data futura
    rows = conn.execute(
        "SELECT indicator_id, ref_date, value::text FROM mart.indicator_values"
        " ORDER BY indicator_id, ref_date"
    ).fetchall()
    assert rows == [
        ("ipca", date(2026, 8, 1), "-0.32"),
        ("ipca", date(2026, 9, 1), "0.82"),
        ("selic", date(2026, 11, 7), "13.75"),
        ("usd_brl", date(2026, 11, 5), "5.4621"),
        ("usd_brl", date(2026, 11, 6), "5.5013"),  # numeric exato, sem 5.50129999...
    ]

    indicators = conn.execute(
        "SELECT id, unit, source_code, source_run_id FROM mart.indicators ORDER BY id"
    ).fetchall()
    assert indicators == [
        ("ipca", "% a.m.", "433", run_id),
        ("selic", "% a.a.", "432", run_id),
        ("usd_brl", "R$/US$", "1", run_id),
    ]


def test_job_is_idempotent(conn):
    insert_run(conn, LATE_NIGHT, VALID_RESPONSES)

    first = job.run(conn)
    second = job.run(conn)

    assert first.mart_rows == second.mart_rows == 5
    assert count(conn, "SELECT count(*) FROM mart.indicator_values") == 5


def test_invalid_data_does_not_touch_mart(conn):
    good_run = insert_run(conn, LATE_NIGHT, VALID_RESPONSES)
    job.run(conn)

    # Coleta mais nova com IPCA de 150% ao mês: viola o contrato da mart.
    insert_run(
        conn,
        datetime(2026, 11, 8, 12, 0, tzinfo=UTC),
        [(433, 200, [obs("01/10/2026", "150")])],
    )
    with pytest.raises(SchemaErrors):
        job.run(conn)

    # A mart continua com os dados da coleta anterior, intactos.
    assert count(conn, "SELECT count(*) FROM mart.indicator_values") == 5
    assert count(conn, "SELECT min(source_run_id) FROM mart.indicators") == good_run


def test_big_jump_is_loaded_and_recorded_as_a_warning(conn):
    insert_run(conn, LATE_NIGHT, VALID_RESPONSES)
    assert run_job(conn, "bcb", job.run)

    # Coleta seguinte com o dólar 12,7% acima do último valor: pode ser real,
    # então entra na mart, mas fica um aviso no registro da execução.
    insert_run(
        conn,
        datetime(2026, 11, 9, 20, 0, tzinfo=BRT),
        [(1, 200, [obs("06/11/2026", "5.5013"), obs("09/11/2026", "6.2")])],
    )
    assert run_job(conn, "bcb", job.run)

    assert count(conn, "SELECT count(*) FROM mart.indicator_values WHERE value = 6.2") == 1
    warnings = conn.execute(
        "SELECT warnings FROM mart.pipeline_runs WHERE job = 'bcb' ORDER BY id DESC LIMIT 1"
    ).fetchone()[0]
    assert warnings == [
        "Dólar comercial (venda): de 5,5013 em 06/11/2026 para 6,2 em 09/11/2026, "
        "variação de +12,7% (limite: 10%)"
    ]
