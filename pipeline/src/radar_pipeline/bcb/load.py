"""Gravação em staging e mart."""

import polars as pl
import psycopg

from radar_pipeline.bcb.indicators import INDICATORS
from radar_pipeline.pg import copy_frame

_UPSERT_INDICATOR = """
INSERT INTO mart.indicators
    (id, name, unit, frequency, source, source_code, source_run_id, updated_at)
VALUES (%s, %s, %s, %s, 'BCB', %s, %s, now())
ON CONFLICT (id) DO UPDATE SET
    name          = EXCLUDED.name,
    unit          = EXCLUDED.unit,
    frequency     = EXCLUDED.frequency,
    source        = EXCLUDED.source,
    source_code   = EXCLUDED.source_code,
    source_run_id = EXCLUDED.source_run_id,
    updated_at    = EXCLUDED.updated_at
"""


def replace_all(
    conn: psycopg.Connection, staging: pl.DataFrame, mart_values: pl.DataFrame, run_id: int
) -> None:
    """Substitui o conteúdo de staging e mart numa única transação.

    Por causa do MVCC do Postgres, quem lê a mart durante a carga (a API)
    continua vendo a versão anterior completa até o COMMIT, nunca uma
    tabela pela metade. Se algo falhar, o ROLLBACK desfaz tudo.
    """
    with conn.transaction(), conn.cursor() as cur:
        cur.execute("DELETE FROM staging.bcb_sgs_observations")
        copy_frame(cur, ("staging", "bcb_sgs_observations"), staging)

        cur.executemany(
            _UPSERT_INDICATOR,
            [(i.id, i.name, i.unit, i.frequency, str(i.sgs_code), run_id) for i in INDICATORS],
        )
        cur.execute("DELETE FROM mart.indicator_values")
        copy_frame(cur, ("mart", "indicator_values"), mart_values)

        # Depois de trocar a tabela inteira, as estatísticas do planejador
        # ficam desatualizadas até o autovacuum passar. ANALYZE resolve na hora.
        cur.execute("ANALYZE staging.bcb_sgs_observations, mart.indicator_values")
