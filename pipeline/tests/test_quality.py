from datetime import date

import polars as pl

from radar_pipeline.anp import quality as anp_quality
from radar_pipeline.bcb import quality as bcb_quality
from radar_pipeline.pg import UFS
from radar_pipeline.text import br_number


def test_br_number():
    assert br_number(1234.5, 2) == "1.234,50"
    assert br_number(62_224, 0) == "62.224"
    assert br_number(12.7, 1, signed=True) == "+12,7"
    assert br_number(-0.32, 2, signed=True) == "-0,32"


# --- BCB ------------------------------------------------------------------------


def values(*rows: tuple[str, date, float]) -> pl.DataFrame:
    return pl.DataFrame(
        rows,
        schema={"indicator_id": pl.String, "ref_date": pl.Date, "value": pl.Float64},
        orient="row",
    )


DOLLAR = values(
    ("usd_brl", date(2026, 11, 5), 5.4621),
    ("usd_brl", date(2026, 11, 6), 5.5013),
    ("usd_brl", date(2026, 11, 9), 6.2),  # +12,7% num dia
)


def test_big_dollar_jump_in_new_values_is_a_warning():
    warnings = bcb_quality.check_steps(DOLLAR, {"usd_brl": date(2026, 11, 6)})
    assert warnings == [
        "Dólar comercial (venda): de 5,5013 em 06/11/2026 para 6,2 em 09/11/2026, "
        "variação de +12,7% (limite: 10%)"
    ]


def test_jumps_the_mart_already_had_are_not_repeated():
    assert bcb_quality.check_steps(DOLLAR, {"usd_brl": date(2026, 11, 9)}) == []


def test_first_load_has_nothing_to_compare():
    assert bcb_quality.check_steps(DOLLAR, {}) == []


def test_selic_steps_are_in_percentage_points():
    selic = values(
        ("selic", date(2026, 11, 6), 13.75),
        ("selic", date(2026, 11, 7), 14.25),  # +0,50 p.p.: decisão normal do Copom
        ("selic", date(2026, 11, 8), 16.75),  # +2,50 p.p.: acima do limite
    )
    assert bcb_quality.check_steps(selic, {"selic": date(2026, 11, 6)}) == [
        "Meta Selic: de 14,25 em 07/11/2026 para 16,75 em 08/11/2026, "
        "variação de +2,50 p.p. (limite: 2,00 p.p.)"
    ]


# --- ANP ------------------------------------------------------------------------


def prices(product: str, month: date, n: int) -> list[tuple[str, date]]:
    return [(product, month)] * n


def test_month_with_few_prices_is_a_warning():
    staging = pl.DataFrame(
        prices("gasolina", date(2026, 9, 1), 10)
        + prices("gasolina", date(2026, 10, 1), 10)
        + prices("gasolina", date(2026, 11, 1), 4)  # 40% do mês típico
        + prices("etanol", date(2026, 9, 1), 5)
        + prices("etanol", date(2026, 10, 1), 5),
        schema={"product": pl.String, "collected_on": pl.Date},
        orient="row",
    )
    assert anp_quality.check_month_volumes(staging) == [
        "Gasolina comum em 11/2026: 4 preços, 40% do mês típico (10); "
        "o arquivo da ANP pode estar incompleto"
    ]


def week(product: str, start: date, national: float, n_ufs: int) -> list[tuple]:
    """A média nacional da semana e uma linha para cada uma das n_ufs UFs."""
    rows = [(product, "BR", start, national, 1000)]
    rows += [(product, uf, start, national, 10) for uf in UFS[:n_ufs]]
    return rows


WEEKLY = pl.DataFrame(
    week("gasolina", date(2026, 10, 19), 6.40, 27)
    + week("gasolina", date(2026, 10, 26), 6.50, 27)
    + week("gasolina", date(2026, 11, 2), 7.80, 20),  # +20% e 7 UFs a menos
    schema={
        "product_id": pl.String,
        "scope": pl.String,
        "week_start": pl.Date,
        "avg_price": pl.Float64,
        "n_stations": pl.Int64,
    },
    orient="row",
)


def test_new_week_with_jump_and_missing_ufs():
    warnings = anp_quality.check_new_weeks(WEEKLY, {"gasolina": date(2026, 10, 26)})
    assert warnings == [
        "Gasolina comum, semana de 02/11/2026: média nacional de R$ 6,50 para R$ 7,80 "
        "(+20,0% sobre a semana de 26/10/2026)",
        "Gasolina comum, semana de 02/11/2026: preços de 20 UFs (o normal são 27)",
    ]


def test_weeks_the_mart_already_had_are_not_checked():
    assert anp_quality.check_new_weeks(WEEKLY, {"gasolina": date(2026, 11, 2)}) == []
    assert anp_quality.check_new_weeks(WEEKLY, {}) == []  # primeira carga
