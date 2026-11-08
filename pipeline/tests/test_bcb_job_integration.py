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
    O teste começa com o BCB vazio (raw, staging, mart e registro), para os
    dados reais do banco de desenvolvimento não se misturarem aos do teste.
    """
    with psycopg.connect(os.environ["TEST_DATABASE_URL"], autocommit=True) as conn:
        with conn.transaction(force_rollback=True):
            conn.execute("DELETE FROM mart.pipeline_runs WHERE job = 'bcb'")
            conn.execute("DELETE FROM mart.indicator_values")
            conn.execute("DELETE FROM staging.bcb_sgs_observations")
            conn.execute("DELETE FROM raw.bcb_sgs_responses")
            conn.execute("DELETE FROM raw.collection_runs WHERE source = 'bcb_sgs'")
            yield conn


def insert_run(
    conn,
    started_at: datetime,
    responses: list[tuple[int, int, object]],
    status: str = "success",
) -> int:
    run_id = conn.execute(
        """INSERT INTO raw.collection_runs (source, started_at, finished_at, status)
           VALUES ('bcb_sgs', %s, %s, %s) RETURNING id""",
        (started_at, started_at, status),
    ).fetchone()[0]
    for code, http_status, payload in responses:
        conn.execute(
            """INSERT INTO raw.bcb_sgs_responses
                   (run_id, series_code, request_url, http_status, payload)
               VALUES (%s, %s, 'http://teste', %s, %s::jsonb)""",
            (run_id, code, http_status, json.dumps(payload)),
        )
    return run_id


def obs(data: str, valor: str) -> dict[str, str]:
    return {"data": data, "valor": valor}


# 23:30 em Brasília já é dia 08 em UTC: o "hoje" da coleta tem que ser 07/11.
LATE_NIGHT = datetime(2026, 11, 7, 23, 30, tzinfo=BRT)
# A coleta seguinte, na segunda-feira.
MONDAY = datetime(2026, 11, 9, 20, 0, tzinfo=BRT)

VALID_RESPONSES = [
    (1, 200, [obs("05/11/2026", "5.4621"), obs("06/11/2026", "5.5013")]),
    (1, 404, {"erro": {"statusCode": 404, "detail": "Value(s) not found"}}),
    (433, 200, [obs("01/08/2026", "-0.32"), obs("01/09/2026", "0.82")]),
    (432, 200, [obs("07/11/2026", "13.75"), obs("08/11/2026", "13.75")]),  # 08/11 é futuro
    (9999, 200, [obs("06/11/2026", "1.0")]),  # série fora do catálogo
]


def count(conn, sql: str, *params) -> int:
    return conn.execute(sql, params).fetchone()[0]


def dollar(conn) -> list[tuple[date, str]]:
    return conn.execute(
        "SELECT ref_date, value::text FROM mart.indicator_values"
        " WHERE indicator_id = 'usd_brl' ORDER BY ref_date"
    ).fetchall()


def last_status(conn) -> str:
    return conn.execute(
        "SELECT status FROM mart.pipeline_runs WHERE job = 'bcb' ORDER BY id DESC LIMIT 1"
    ).fetchone()[0]


def test_job_fills_staging_and_mart(conn):
    run_id = insert_run(conn, LATE_NIGHT, VALID_RESPONSES)

    summary = job.run(conn)

    assert summary is not None
    assert summary.source_run_id == run_id
    assert summary.source_collected_at == LATE_NIGHT
    assert summary.as_of == date(2026, 11, 7)
    assert (summary.runs, summary.mart_rows) == (1, 5)

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


def test_new_collection_is_merged_without_deleting_history(conn):
    insert_run(conn, LATE_NIGHT, VALID_RESPONSES)
    assert run_job(conn, "bcb", job.run)

    # Coleta incremental: só a janela recente do dólar, com um valor novo
    # (09/11) e o de 06/11 corrigido pelo BCB.
    newer = insert_run(
        conn, MONDAY, [(1, 200, [obs("06/11/2026", "5.51"), obs("09/11/2026", "5.53")])]
    )
    summary = job.run(conn)

    # Só a coleta nova foi lida; na mart, um valor entrou e um mudou.
    assert (summary.runs, summary.source_run_id, summary.rows_written) == (1, newer, 2)
    assert dollar(conn) == [
        (date(2026, 11, 5), "5.4621"),
        (date(2026, 11, 6), "5.51"),
        (date(2026, 11, 9), "5.53"),
    ]
    # IPCA e Selic não vieram na coleta nova, e continuam na mart.
    others = "SELECT count(*) FROM mart.indicator_values WHERE indicator_id <> 'usd_brl'"
    assert count(conn, others) == 3


def test_without_new_collection_the_job_is_skipped(conn):
    insert_run(conn, LATE_NIGHT, VALID_RESPONSES)
    assert run_job(conn, "bcb", job.run)
    assert last_status(conn) == "success"

    assert run_job(conn, "bcb", job.run)
    assert last_status(conn) == "skipped"


def test_failed_collection_is_ignored(conn):
    insert_run(conn, LATE_NIGHT, VALID_RESPONSES)
    # Uma coleta que falhou pode ter parado no meio: não entra.
    insert_run(conn, MONDAY, [(1, 200, [obs("09/11/2026", "5.53")])], status="failed")

    summary = job.run(conn)

    assert summary.runs == 1
    assert date(2026, 11, 9) not in [d for d, _ in dollar(conn)]


def test_reprocessing_the_same_collection_changes_nothing(conn):
    insert_run(conn, LATE_NIGHT, VALID_RESPONSES)

    first = job.run(conn)
    second = job.run(conn)  # sem o registro do CLI, a mesma coleta é lida de novo

    assert (first.rows_written, second.rows_written) == (5, 0)
    assert count(conn, "SELECT count(*) FROM mart.indicator_values") == 5


def test_full_refresh_rebuilds_from_all_collections(conn):
    insert_run(conn, LATE_NIGHT, VALID_RESPONSES)
    assert run_job(conn, "bcb", job.run)
    insert_run(conn, MONDAY, [(1, 200, [obs("06/11/2026", "5.51")])])
    assert run_job(conn, "bcb", job.run)
    # Um valor que não veio de nenhuma coleta (sobra de uma regra antiga, por exemplo).
    conn.execute("INSERT INTO mart.indicator_values VALUES ('usd_brl', '2026-01-02', 9.99)")

    summary = job.run(conn, full_refresh=True)

    # Todas as coletas foram relidas, e a mart foi refeita só com o que veio delas.
    assert summary.runs == 2
    assert dollar(conn) == [(date(2026, 11, 5), "5.4621"), (date(2026, 11, 6), "5.51")]


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
    insert_run(conn, MONDAY, [(1, 200, [obs("06/11/2026", "5.5013"), obs("09/11/2026", "6.2")])])
    assert run_job(conn, "bcb", job.run)

    assert count(conn, "SELECT count(*) FROM mart.indicator_values WHERE value = 6.2") == 1
    warnings = conn.execute(
        "SELECT warnings FROM mart.pipeline_runs WHERE job = 'bcb' ORDER BY id DESC LIMIT 1"
    ).fetchone()[0]
    assert warnings == [
        "Dólar comercial (venda): de 5,5013 em 06/11/2026 para 6,2 em 09/11/2026, "
        "variação de +12,7% (limite: 10%)"
    ]
