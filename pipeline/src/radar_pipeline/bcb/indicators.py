"""Catálogo dos indicadores publicados na mart."""

from dataclasses import dataclass
from typing import Literal


@dataclass(frozen=True)
class Indicator:
    """Um indicador da mart e a série do SGS que o alimenta.

    min_value (exclusivo) e max_value (inclusivo) são limites de sanidade:
    servem para pegar erro de parsing ou de escala (ex.: 5501.3 em vez de
    5.5013), não são previsões econômicas.
    """

    id: str
    sgs_code: int
    name: str
    unit: str
    frequency: Literal["daily", "monthly"]
    min_value: float
    max_value: float


INDICATORS: tuple[Indicator, ...] = (
    Indicator("selic", 432, "Meta Selic", "% a.a.", "daily", 0, 100),
    Indicator("ipca", 433, "IPCA (variação mensal)", "% a.m.", "monthly", -5, 100),
    Indicator("usd_brl", 1, "Dólar comercial (venda)", "R$/US$", "daily", 0, 100),
)
