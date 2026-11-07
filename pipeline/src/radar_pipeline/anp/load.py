"""Gravação dos preços em staging e mart."""

import polars as pl
import psycopg

from radar_pipeline.anp.products import PRODUCTS
from radar_pipeline.pg import copy_frame

_UPSERT_PRODUCT = """
INSERT INTO mart.fuel_products (id, name, unit, updated_at)
VALUES (%s, %s, %s, now())
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name, unit = EXCLUDED.unit, updated_at = EXCLUDED.updated_at
"""


def replace_all(
    conn: psycopg.Connection, staging: pl.DataFrame, weekly: pl.DataFrame, municipal: pl.DataFrame
) -> None:
    """Substitui staging e mart dos combustíveis numa única transação.

    O staging é esvaziado com TRUNCATE: com ~800 mil linhas, é bem mais rápido
    que DELETE e não deixa versões mortas para o autovacuum limpar. O TRUNCATE
    bloqueia leituras até o fim da transação, o que não importa no staging,
    que ninguém lê. Já a mart é lida pela API: lá vai DELETE, e o MVCC faz a
    API continuar vendo a versão anterior até o COMMIT.
    """
    with conn.transaction(), conn.cursor() as cur:
        cur.execute("TRUNCATE staging.fuel_prices")
        copy_frame(cur, ("staging", "fuel_prices"), staging)

        cur.executemany(_UPSERT_PRODUCT, [(p.id, p.name, p.unit) for p in PRODUCTS])

        cur.execute("DELETE FROM mart.fuel_prices_weekly")
        copy_frame(cur, ("mart", "fuel_prices_weekly"), weekly)

        cur.execute("DELETE FROM mart.fuel_prices_municipal")
        copy_frame(cur, ("mart", "fuel_prices_municipal"), municipal)

        cur.execute(
            "ANALYZE staging.fuel_prices, mart.fuel_prices_weekly, mart.fuel_prices_municipal"
        )
