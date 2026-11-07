"""Catálogo dos combustíveis publicados na mart."""

from dataclasses import dataclass


@dataclass(frozen=True)
class Product:
    """Um combustível, como a ANP o escreve e como a mart o publica.

    min_price (exclusivo) e max_price (inclusivo) são limites de sanidade:
    pegam erro de escala (699 em vez de 6,99) ou preço zerado, não são
    previsões. Os preços reais de nov/2025 a out/2026 ficaram entre
    R$ 2,77 (etanol) e R$ 9,99 (diesel S10).
    """

    id: str
    anp_name: str  # valor da coluna "Produto" no CSV
    anp_unit: str  # valor da coluna "Unidade de Medida"
    name: str
    unit: str
    min_price: float
    max_price: float


PRODUCTS: tuple[Product, ...] = (
    Product("gasolina", "GASOLINA", "R$ / litro", "Gasolina comum", "R$/l", 2, 20),
    Product(
        "gasolina_aditivada",
        "GASOLINA ADITIVADA",
        "R$ / litro",
        "Gasolina aditivada",
        "R$/l",
        2,
        20,
    ),
    Product("etanol", "ETANOL", "R$ / litro", "Etanol hidratado", "R$/l", 1, 15),
    Product("diesel", "DIESEL", "R$ / litro", "Diesel comum (S500)", "R$/l", 2, 20),
    Product("diesel_s10", "DIESEL S10", "R$ / litro", "Diesel S10", "R$/l", 2, 20),
    Product("gnv", "GNV", "R$ / m³", "Gás natural veicular", "R$/m³", 1, 15),
)

PRODUCT_IDS = [p.id for p in PRODUCTS]
