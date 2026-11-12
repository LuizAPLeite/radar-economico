"""Gravação em staging e mart."""

import polars as pl
import psycopg

from radar_pipeline.bcb.indicators import INDICATORS
from radar_pipeline.pg import copy_frame, upsert_frame

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


def _upsert_indicators(cur: psycopg.Cursor, run_id: int) -> None:
    cur.executemany(
        _UPSERT_INDICATOR,
        [(i.id, i.name, i.unit, i.frequency, str(i.sgs_code), run_id) for i in INDICATORS],
    )


def upsert(
    conn: psycopg.Connection, staging: pl.DataFrame, mart_values: pl.DataFrame, run_id: int
) -> int:
    """Grava as coletas novas sem apagar nada, numa transação: datas novas
    entram, valores corrigidos são atualizados, o resto fica como estava.

    Assim, uma coleta que voltar incompleta (uma janela vazia, por exemplo)
    não tem como apagar histórico da mart. Devolve quantos valores entraram
    ou mudaram na mart.
    """
    with conn.transaction(), conn.cursor() as cur:
        upsert_frame(cur, ("staging", "bcb_sgs_observations"), staging, ["series_code", "ref_date"])
        _upsert_indicators(cur, run_id)
        return upsert_frame(
            cur, ("mart", "indicator_values"), mart_values, ["indicator_id", "ref_date"]
        )


def replace_all(
    conn: psycopg.Connection, staging: pl.DataFrame, mart_values: pl.DataFrame, run_id: int
) -> int:
    """Substitui o conteúdo de staging e mart numa única transação (carga
    completa, com PIPELINE_FULL_REFRESH). Devolve quantos valores a mart tem.

    Por causa do MVCC do Postgres, quem lê a mart durante a carga (a API)
    continua vendo a versão anterior completa até o COMMIT, nunca uma
    tabela pela metade. Se algo falhar, o ROLLBACK desfaz tudo.
    """
    with conn.transaction(), conn.cursor() as cur:
        cur.execute("DELETE FROM staging.bcb_sgs_observations")
        copy_frame(cur, ("staging", "bcb_sgs_observations"), staging)

        _upsert_indicators(cur, run_id)
        # Só os indicadores do BCB: a tabela também guarda os de outras
        # fontes (o Brent, do FRED), que este job não reconstrói.
        cur.execute(
            "DELETE FROM mart.indicator_values WHERE indicator_id = ANY(%s)",
            ([i.id for i in INDICATORS],),
        )
        copy_frame(cur, ("mart", "indicator_values"), mart_values)

        # Depois de trocar a tabela inteira, as estatísticas do planejador
        # ficam desatualizadas até o autovacuum passar. ANALYZE resolve na hora.
        cur.execute("ANALYZE staging.bcb_sgs_observations, mart.indicator_values")
    return mart_values.height
