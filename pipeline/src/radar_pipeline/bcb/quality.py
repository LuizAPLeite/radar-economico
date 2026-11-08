"""Checagens de qualidade do BCB que viram avisos (não impedem a carga).

O contrato (pandera) barra o que está claramente errado, como uma Selic de
1375%. Aqui ficam os sinais de que algo PODE estar errado, mas também pode
ser real: um salto grande demais entre dois valores seguidos. Eles vão para
o registro da execução (mart.pipeline_runs.warnings), para alguém conferir.

Só os valores novos de cada carga são olhados (datas depois do último valor
que a mart já tinha): o histórico foi conferido nas cargas anteriores, e
repetir a cada execução os saltos da hiperinflação dos anos 1990 seria ruído.
"""

import itertools
from datetime import date

import polars as pl

from radar_pipeline.bcb.indicators import INDICATORS, Indicator
from radar_pipeline.text import br_number


def check_steps(mart_values: pl.DataFrame, previous_last: dict[str, date]) -> list[str]:
    """Saltos acima de max_step entre valores seguidos, nos valores novos.

    previous_last tem a data do último valor de cada indicador na mart antes
    desta carga. Esse último valor antigo entra na comparação com o primeiro
    novo. Indicador sem valor anterior (primeira carga) não é checado: não há
    com o que comparar.
    """
    warnings = []
    for indicator in INDICATORS:
        last = previous_last.get(indicator.id)
        if last is None:
            continue
        rows = (
            mart_values.filter(
                (pl.col("indicator_id") == indicator.id) & (pl.col("ref_date") >= last)
            )
            .sort("ref_date")
            .select("ref_date", "value")
            .rows()
        )
        # pairwise: (v1, v2), (v2, v3), (v3, v4)... cada valor com o seguinte.
        for (d0, v0), (d1, v1) in itertools.pairwise(rows):
            if _step(indicator, v0, v1) > indicator.max_step:
                warnings.append(_describe(indicator, d0, v0, d1, v1))
    return warnings


def _step(indicator: Indicator, before: float, after: float) -> float:
    if indicator.relative_step:
        # O contrato garante valor > 0 nos indicadores com salto relativo.
        return abs(after / before - 1)
    return abs(after - before)


def _describe(indicator: Indicator, d0: date, v0: float, d1: date, v1: float) -> str:
    if indicator.relative_step:
        change = br_number((v1 / v0 - 1) * 100, 1, signed=True) + "%"
        limit = br_number(indicator.max_step * 100, 0) + "%"
    else:
        change = br_number(v1 - v0, 2, signed=True) + " p.p."
        limit = br_number(indicator.max_step, 2) + " p.p."
    # str(5.5013) = "5.5013": a menor escrita exata do número, só trocando o ponto.
    before, after = str(v0).replace(".", ","), str(v1).replace(".", ",")
    return (
        f"{indicator.name}: de {before} em {d0:%d/%m/%Y} para {after} em {d1:%d/%m/%Y}, "
        f"variação de {change} (limite: {limit})"
    )
