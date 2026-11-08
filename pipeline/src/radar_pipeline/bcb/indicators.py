"""Catálogo dos indicadores publicados na mart."""

from dataclasses import dataclass
from typing import Literal


@dataclass(frozen=True)
class Indicator:
    """Um indicador da mart e a série do SGS que o alimenta.

    min_value (exclusivo) e max_value (inclusivo) são limites de sanidade:
    servem para pegar erro de parsing ou de escala (ex.: 5501.3 em vez de
    5.5013), não são previsões econômicas. Violá-los faz o job falhar.

    max_step é a maior variação entre dois valores seguidos que passa sem
    aviso (ver quality.py): acima dela, pode ser erro na fonte, mas também
    pode ser real, por isso só gera um aviso. É uma fração do valor anterior
    quando relative_step é True (dólar), ou pontos percentuais (taxas).
    """

    id: str
    sgs_code: int
    name: str
    unit: str
    frequency: Literal["daily", "monthly"]
    min_value: float
    max_value: float
    max_step: float
    relative_step: bool = False


# Os limites de salto ficam acima dos maiores movimentos reais desde 2000: o
# dólar já variou 9,3% num dia, e a Selic mudou 1,5 p.p. numa reunião.
INDICATORS: tuple[Indicator, ...] = (
    Indicator("selic", 432, "Meta Selic", "% a.a.", "daily", 0, 100, max_step=2.0),
    Indicator("ipca", 433, "IPCA (variação mensal)", "% a.m.", "monthly", -5, 100, max_step=2.0),
    Indicator(
        "usd_brl",
        1,
        "Dólar comercial (venda)",
        "R$/US$",
        "daily",
        0,
        100,
        max_step=0.10,
        relative_step=True,
    ),
)
