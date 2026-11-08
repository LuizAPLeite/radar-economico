"""Integração dos jobs do IBGE e da ANP num Postgres de verdade.

Tudo roda numa transação que SEMPRE sofre rollback (ver a fixture `conn`):
o banco de desenvolvimento termina exatamente como estava.
"""

import gzip
import json
import os
from datetime import date

import psycopg
import pytest

from radar_pipeline.anp import job as anp_job
from radar_pipeline.anp.transform import SourceFormatError
from radar_pipeline.ibge import job as ibge_job

pytestmark = pytest.mark.skipif(
    not os.environ.get("TEST_DATABASE_URL"), reason="TEST_DATABASE_URL não definida"
)

HEADER = (
    "﻿Regiao - Sigla;Estado - Sigla;Municipio;Revenda;CNPJ da Revenda;Nome da Rua;"
    "Numero Rua;Complemento;Bairro;Cep;Produto;Data da Coleta;Valor de Venda;"
    "Valor de Compra;Unidade de Medida;Bandeira"
)

REAL_MUNICIPALITIES = [
    (3518800, "Guarulhos", "SP"),
    (4317103, "Sant'Ana do Livramento", "RS"),
    (2903201, "Barreiras", "BA"),
]


@pytest.fixture
def conn():
    with psycopg.connect(os.environ["TEST_DATABASE_URL"], autocommit=True) as conn:
        with conn.transaction(force_rollback=True):
            yield conn


def insert_run(conn, source: str) -> int:
    return conn.execute(
        "INSERT INTO raw.collection_runs (source, status) VALUES (%s, 'success') RETURNING id",
        (source,),
    ).fetchone()[0]


def insert_ibge(conn) -> None:
    # O job exige ≥ 5.000 municípios: 5.000 fictícios + os reais usados nos preços.
    fake = [(1_100_000 + i, f"Municipio {i}", "RO") for i in range(5_000)]
    municipios = [
        {"municipio-id": code, "municipio-nome": name, "UF-sigla": uf}
        for code, name, uf in fake + REAL_MUNICIPALITIES
    ]
    centroides = [
        {"id": str(code), "centroide": {"latitude": -10.0, "longitude": -50.0}}
        for code, _, _ in fake + REAL_MUNICIPALITIES
    ]
    run_id = insert_run(conn, "ibge")
    for dataset, payload in (("municipios", municipios), ("centroides", centroides)):
        conn.execute(
            """INSERT INTO raw.ibge_responses (run_id, dataset, request_url, sha256, payload)
               VALUES (%s, %s, 'http://teste', 'teste', %s::jsonb)""",
            (run_id, dataset, json.dumps(payload)),
        )


def insert_anp_file(conn, rows: list[str]) -> None:
    content = ("\r\n".join([HEADER, *rows]) + "\r\n").encode()
    conn.execute(
        """INSERT INTO raw.anp_files
               (run_id, product_group, ref_month, url, sha256, size_bytes, content_gzip)
           VALUES (%s, 'gasolina-etanol', '2026-10-01', 'http://teste', %s, %s, %s)""",
        (insert_run(conn, "anp"), f"teste-{len(rows)}", len(content), gzip.compress(content)),
    )


def row(uf, municipio, cnpj, produto, data, preco) -> str:
    return (
        f"SE;{uf};{municipio};POSTO; {cnpj};RUA;1;;CENTRO;00000-000;"
        f"{produto};{data};{preco};;R$ / litro;VIBRA"
    )


VALID_ROWS = [
    row("SP", "GUARULHOS", "11.111.111/0001-11", "GASOLINA", "06/10/2026", "6,00"),
    row("SP", "GUARULHOS", "11.111.111/0001-11", "GASOLINA", "06/10/2026", "6,00"),  # cópia
    row("SP", "GUARULHOS", "22.222.222/0001-22", "GASOLINA", "07/10/2026", "7,00"),
    row("RS", "SANTANA DO LIVRAMENTO", "33.333.333/0001-33", "GASOLINA", "08/10/2026", "6,50"),
    row("BA", "BARREIRAS", "44.444.444/0001-44", "ETANOL", "08/10/2026", "4,49"),
]


def fetch(conn, sql: str):
    return conn.execute(sql).fetchall()


def test_ibge_and_anp_jobs_fill_staging_and_mart(conn):
    conn.execute("DELETE FROM raw.anp_files")  # só o arquivo do teste (o rollback desfaz)
    insert_ibge(conn)
    insert_anp_file(conn, VALID_ROWS)

    ibge = ibge_job.run(conn)
    assert ibge.municipalities == 5_003 and ibge.with_coordinates == 5_003
    # Rodar de novo com o mesmo raw não reescreve nenhum município na mart.
    assert ibge_job.run(conn).rows_written == 0

    anp = anp_job.run(conn)
    assert (anp.files, anp.staging_rows, anp.duplicates_removed) == (1, 4, 1)
    assert anp.rows_written == anp.weekly_rows + anp.municipal_rows

    assert fetch(
        conn, "SELECT product, ibge_code, price::text FROM staging.fuel_prices ORDER BY cnpj"
    ) == [
        # O preço passa por Float64: "6,00" vira 6.0 (mesmo valor, outra escrita).
        ("gasolina", 3518800, "6.0"),
        ("gasolina", 3518800, "7.0"),
        ("gasolina", 4317103, "6.5"),  # casou pela tabela de exceções
        ("etanol", 2903201, "4.49"),
    ]

    # Semana de 05/10/2026: gasolina no Brasil = média de 6,00; 7,00 e 6,50.
    assert fetch(
        conn,
        """SELECT scope, week_start, avg_price::text, n_stations FROM mart.fuel_prices_weekly
            WHERE product_id = 'gasolina' ORDER BY scope""",
    ) == [
        ("BR", date(2026, 10, 5), "6.5", 3),
        ("RS", date(2026, 10, 5), "6.5", 1),
        ("SP", date(2026, 10, 5), "6.5", 2),
    ]

    assert fetch(
        conn,
        """SELECT ibge_code, avg_price::text, min_price::text, max_price::text, n_stations
             FROM mart.fuel_prices_municipal WHERE product_id = 'gasolina' ORDER BY ibge_code""",
    ) == [
        (3518800, "6.5", "6.0", "7.0", 2),
        (4317103, "6.5", "6.5", "6.5", 1),
    ]

    assert len(fetch(conn, "SELECT id FROM mart.fuel_products")) == 6


def test_invalid_anp_file_does_not_touch_the_mart(conn):
    conn.execute("DELETE FROM raw.anp_files")
    insert_ibge(conn)
    insert_anp_file(conn, VALID_ROWS)
    ibge_job.run(conn)
    anp_job.run(conn)

    # Versão nova do mesmo arquivo com um produto que o catálogo não conhece.
    insert_anp_file(
        conn, [row("SP", "GUARULHOS", "11.111.111/0001-11", "HIDROGENIO", "09/10/2026", "9,00")]
    )
    with pytest.raises(SourceFormatError, match="HIDROGENIO"):
        anp_job.run(conn)

    # A mart continua com os dados da carga anterior, intactos.
    assert fetch(conn, "SELECT count(*) FROM staging.fuel_prices") == [(4,)]
    assert fetch(conn, "SELECT count(*) FROM mart.fuel_prices_weekly WHERE scope = 'BR'") == [(2,)]
