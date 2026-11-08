"""Checagens de qualidade da ANP que viram avisos (não impedem a carga).

Como no BCB (ver bcb/quality.py): o contrato barra o que está claramente
errado, e estes avisos marcam o que pode ser erro mas também pode ser real.
Os limites foram calibrados com os dados de nov/2025 a out/2026, para não
avisar sobre o que é normal na pesquisa.
"""

from datetime import date

import polars as pl

from radar_pipeline.anp.products import PRODUCTS
from radar_pipeline.text import br_number

_NAMES = {p.id: p.name for p in PRODUCTS}

# Um mês com bem menos preços que o mês típico do produto sugere arquivo
# incompleto. Os meses reais ficaram entre 86% e 108% da mediana.
MIN_MONTH_SHARE = 0.7

# Variação da média nacional de uma semana para a seguinte. Movimentos reais
# grandes existem (o diesel subiu 11% numa semana em 2026), por isso o limite
# fica bem acima deles: o que passar disso tende a ser erro de escala ou unidade.
MAX_WEEKLY_CHANGE = 0.15

# UFs a menos que o normal do produto numa semana. Algumas UFs pequenas ficam
# sem pesquisa em semanas comuns (a gasolina oscila entre 26 e 27), e o GNV só
# existe em ~16 UFs; por isso a referência é a mediana de cada produto.
UF_SLACK = 2


def check_month_volumes(staging: pl.DataFrame) -> list[str]:
    """Meses em que um produto tem menos de MIN_MONTH_SHARE do seu mês típico."""
    low = (
        staging.group_by("product", pl.col("collected_on").dt.truncate("1mo").alias("month"))
        .len("prices")
        .with_columns(typical=pl.col("prices").median().over("product"))
        .filter(pl.col("prices") < pl.col("typical") * MIN_MONTH_SHARE)
        .sort("product", "month")
    )
    return [
        f"{_NAMES[product]} em {month:%m/%Y}: {br_number(prices, 0)} preços, "
        f"{prices / typical:.0%} do mês típico ({br_number(typical, 0)}); "
        "o arquivo da ANP pode estar incompleto"
        for product, month, prices, typical in low.select(
            "product", "month", "prices", "typical"
        ).rows()
    ]


def check_new_weeks(weekly: pl.DataFrame, previous_last: dict[str, date]) -> list[str]:
    """Semanas que a mart ainda não tinha: salto na média nacional e UFs faltando.

    previous_last tem a semana mais recente de cada produto na mart antes desta
    carga. Produto sem semana anterior (primeira carga) não é checado: não há
    com o que comparar, e o histórico inteiro geraria avisos repetidos.
    """
    ufs = (
        weekly.filter(pl.col("scope") != "BR")
        .group_by("product_id", "week_start")
        .len("n_ufs")
        .with_columns(typical=pl.col("n_ufs").median().over("product_id"))
    )
    national = (
        weekly.filter(pl.col("scope") == "BR")
        .sort("product_id", "week_start")
        .with_columns(
            # shift().over(): o valor da linha anterior do mesmo produto.
            prev_week=pl.col("week_start").shift().over("product_id"),
            prev_price=pl.col("avg_price").shift().over("product_id"),
        )
        .join(ufs, on=["product_id", "week_start"], how="left")
    )

    warnings = []
    for row in national.iter_rows(named=True):
        last = previous_last.get(row["product_id"])
        if last is None or row["week_start"] <= last:
            continue  # semana que a mart já tinha
        label = f"{_NAMES[row['product_id']]}, semana de {row['week_start']:%d/%m/%Y}"

        if row["prev_price"] is not None:
            change = row["avg_price"] / row["prev_price"] - 1
            if abs(change) > MAX_WEEKLY_CHANGE:
                warnings.append(
                    f"{label}: média nacional de R$ {br_number(row['prev_price'], 2)} "
                    f"para R$ {br_number(row['avg_price'], 2)} "
                    f"({br_number(change * 100, 1, signed=True)}% sobre a semana de "
                    f"{row['prev_week']:%d/%m/%Y})"
                )

        if row["n_ufs"] is not None and row["n_ufs"] < row["typical"] - UF_SLACK:
            warnings.append(
                f"{label}: preços de {row['n_ufs']} UFs (o normal são "
                f"{br_number(row['typical'], 0)})"
            )
    return warnings
