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
