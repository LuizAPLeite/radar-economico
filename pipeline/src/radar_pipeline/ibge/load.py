"""Gravação dos municípios em staging e mart."""

import polars as pl
import psycopg

from radar_pipeline.pg import copy_frame

# Os preços da mart apontam para mart.municipalities (chave estrangeira),
# então a tabela é atualizada linha a linha em vez de apagada e regravada.
# O WHERE evita reescrever (e inchar a tabela com versões mortas) as linhas
# que não mudaram, que são quase todas a cada execução.
_UPSERT_MUNICIPALITY = """
INSERT INTO mart.municipalities (ibge_code, name, uf, latitude, longitude)
VALUES (%s, %s, %s, %s, %s)
ON CONFLICT (ibge_code) DO UPDATE SET
    name = EXCLUDED.name, uf = EXCLUDED.uf,
    latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude
WHERE (mart.municipalities.name, mart.municipalities.uf,
       mart.municipalities.latitude, mart.municipalities.longitude)
      IS DISTINCT FROM
      (EXCLUDED.name, EXCLUDED.uf, EXCLUDED.latitude, EXCLUDED.longitude)
"""


def replace(conn: psycopg.Connection, municipalities: pl.DataFrame) -> None:
    """Grava staging (todos) e mart (só os que têm coordenadas) numa transação."""
    with conn.transaction(), conn.cursor() as cur:
        cur.execute("DELETE FROM staging.municipalities")
        copy_frame(cur, ("staging", "municipalities"), municipalities)

        with_coordinates = municipalities.drop_nulls(["latitude", "longitude"])
        # executemany manda todas as linhas de uma vez (pipeline do psycopg),
        # sem esperar a resposta de cada INSERT.
        cur.executemany(_UPSERT_MUNICIPALITY, with_coordinates.rows())

        cur.execute("ANALYZE staging.municipalities, mart.municipalities")
