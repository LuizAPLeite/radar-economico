"""Períodos da política de preços da Petrobras, que mudam a relação entre o
petróleo e o preço na bomba. As análises são feitas separadamente em cada um."""

from dataclasses import dataclass
from datetime import date


@dataclass(frozen=True)
class Regime:
    id: str
    name: str
    starts_on: date
    ends_on: date | None  # None: período atual (o fim é exclusivo)
    description: str


REGIMES: tuple[Regime, ...] = (
    Regime(
        "controle",
        "Preços administrados (2004–2016)",
        date(2004, 5, 1),
        date(2016, 10, 14),
        "A Petrobras reajustava raramente e com frequência segurou preços abaixo do mercado "
        "internacional.",
    ),
    Regime(
        "ppi",
        "Paridade internacional (2016–2023)",
        # Anúncio da nova política em 14/10/2016; reajustes diários a partir de jul/2017.
        date(2016, 10, 14),
        date(2023, 5, 16),
        "Preços acompanhando o mercado internacional (PPI): petróleo e câmbio em reais.",
    ),
    Regime(
        "atual",
        "Estratégia comercial atual (desde 2023)",
        # Fim do PPI anunciado em 16/05/2023.
        date(2023, 5, 16),
        None,
        "A Petrobras deixou a paridade formal, mas segue o mercado internacional com "
        "reajustes menos frequentes.",
    ),
)

CURRENT = REGIMES[-1]
