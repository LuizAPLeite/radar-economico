"""Ajudantes de banco compartilhados pelos jobs."""

import polars as pl
import psycopg
from psycopg import sql

# As 27 UFs, usadas nas validações.
UFS = (
    "AC", "AL", "AM", "AP", "BA", "CE", "DF", "ES", "GO", "MA", "MG", "MS", "MT", "PA",
    "PB", "PE", "PI", "PR", "RJ", "RN", "RO", "RR", "RS", "SC", "SE", "SP", "TO",
)  # fmt: skip


def copy_frame(cur: psycopg.Cursor, table: tuple[str, str], df: pl.DataFrame) -> None:
    """Insere o DataFrame na tabela com COPY ... FROM STDIN (FORMAT csv).

    O polars escreve o CSV em memória de uma vez, rápido mesmo com centenas de
    milhares de linhas, e o Postgres converte cada coluna para o tipo da
    tabela. Nulos viram campo vazio, que no formato CSV do COPY é NULL.
    Nomes de tabela e coluna entram via sql.Identifier, que os escapa.
    """
    stmt = sql.SQL("COPY {table} ({columns}) FROM STDIN (FORMAT csv)").format(
        table=sql.Identifier(*table),
        columns=sql.SQL(", ").join(sql.Identifier(c) for c in df.columns),
    )
    with cur.copy(stmt) as copy:
        copy.write(df.write_csv(include_header=False).encode())


def upsert_frame(
    cur: psycopg.Cursor, table: tuple[str, str], df: pl.DataFrame, key: list[str]
) -> int:
    """Insere as linhas novas e atualiza as que mudaram. Devolve quantas.

    Carga em lote em duas etapas: COPY (rápido) para uma tabela temporária
    com a mesma estrutura, e dela um único INSERT ... ON CONFLICT na tabela de
    destino. Uma linha cuja chave (key) já existe vira UPDATE; o WHERE pula as
    que não mudaram, que assim não são reescritas nem contadas. Nada é apagado.
    """
    others = [c for c in df.columns if c not in key]
    temp = sql.Identifier("pg_temp", f"upsert_{table[1]}")
    target = sql.Identifier(*table)
    columns = sql.SQL(", ").join(sql.Identifier(c) for c in df.columns)

    # ON COMMIT DROP é só uma garantia: a tabela é apagada logo no fim, para
    # o mesmo upsert poder rodar de novo dentro da mesma transação.
    cur.execute(
        sql.SQL("CREATE TEMP TABLE {temp} (LIKE {target}) ON COMMIT DROP").format(
            temp=temp, target=target
        )
    )
    copy_frame(cur, ("pg_temp", f"upsert_{table[1]}"), df)
    cur.execute(
        sql.SQL(
            """
            INSERT INTO {target} AS t ({columns})
            SELECT {columns} FROM {temp}
            ON CONFLICT ({key}) DO UPDATE SET {updates}
            WHERE ({current}) IS DISTINCT FROM ({incoming})
            """
        ).format(
            target=target,
            columns=columns,
            temp=temp,
            key=sql.SQL(", ").join(sql.Identifier(c) for c in key),
            updates=sql.SQL(", ").join(
                sql.SQL("{c} = EXCLUDED.{c}").format(c=sql.Identifier(c)) for c in others
            ),
            current=sql.SQL(", ").join(sql.Identifier("t", c) for c in others),
            incoming=sql.SQL(", ").join(sql.Identifier("excluded", c) for c in others),
        )
    )
    written = cur.rowcount
    cur.execute(sql.SQL("DROP TABLE {temp}").format(temp=temp))
    return written
