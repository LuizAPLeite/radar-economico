"""Testes das transformações da série histórica da ANP (sem banco)."""

import io
from datetime import date, datetime

import polars as pl
import pytest
import xlsxwriter
from pandera.errors import SchemaError, SchemaErrors

from radar_pipeline.anp.transform import SourceFormatError
from radar_pipeline.anp_serie import transform

HEADER = [
    "DATA INICIAL",
    "DATA FINAL",
    "PRODUTO",
    "NÚMERO DE POSTOS PESQUISADOS",
    "UNIDADE DE MEDIDA",
    "PREÇO MÉDIO REVENDA",
    "DESVIO PADRÃO REVENDA",
]


def make_sheet(rows: list[list], notes: int = 3, header: list[str] = HEADER) -> bytes:
    """Uma planilha no formato da ANP: observações no topo, linha em branco,
    cabeçalho e dados (datas como data do Excel)."""
    buffer = io.BytesIO()
    workbook = xlsxwriter.Workbook(buffer, {"in_memory": True})
    sheet = workbook.add_worksheet("BRASIL")
    date_format = workbook.add_format({"num_format": "dd/mm/yyyy"})
    r = 0
    for i in range(notes):
        sheet.write(r, 0, f"OBSERVAÇÃO {i}")
        r += 1
    r += 1  # linha em branco
    sheet.write_row(r, 0, header)
    for row in rows:
        r += 1
        for c, value in enumerate(row):
            if isinstance(value, datetime):
                sheet.write_datetime(r, c, value, date_format)
            else:
                sheet.write(r, c, value)
    workbook.close()
    return buffer.getvalue()


def row(start: datetime, product: str, unit: str, price: float, n: int = 100) -> list:
    return [start, start, product, n, unit, price, 0.1]


SUNDAY = datetime(2026, 10, 25)
NEXT_SUNDAY = datetime(2026, 11, 1)


def test_parse_finds_header_after_notes_of_any_length():
    for notes in (1, 7):
        frame = transform.parse_sheet(
            make_sheet([row(SUNDAY, "GASOLINA COMUM", "R$/l", 6.5, 9000)], notes=notes)
        )
        assert frame.to_dicts() == [
            {
                "week_start_anp": date(2026, 10, 25),
                "product_anp": "GASOLINA COMUM",
                "n_stations": 9000,
                "unit": "R$/l",
                "avg_price": 6.5,
            }
        ]


def test_parse_fails_without_header_or_columns():
    with pytest.raises(SourceFormatError, match="cabeçalho"):
        transform.parse_sheet(make_sheet([], header=["OUTRA COISA"]))
    with pytest.raises(SourceFormatError, match="PREÇO MÉDIO REVENDA"):
        transform.parse_sheet(make_sheet([], header=HEADER[:5]))


def parsed(rows: list[list]) -> pl.DataFrame:
    return transform.parse_sheet(make_sheet(rows))


def test_to_mart_maps_products_moves_week_to_monday_and_ignores_glp():
    old = parsed([row(datetime(2012, 12, 23), "ÓLEO DIESEL", "R$/l", 2.15)])
    new = parsed(
        [
            row(SUNDAY, "OLEO DIESEL", "R$/l", 6.9),
            row(SUNDAY, "GNV", "R$/m³", 4.8),
            row(NEXT_SUNDAY, "GNV", "R$/m3", 4.9),
            row(SUNDAY, "GLP", "R$/13kg", 110.0),
        ]
    )
    mart = transform.to_mart([old, new])
    assert mart.select("product_id", "week_start", "avg_price").rows() == [
        ("diesel", date(2012, 12, 24), 2.15),
        ("diesel", date(2026, 10, 26), 6.9),
        ("gnv", date(2026, 10, 26), 4.8),
        ("gnv", date(2026, 11, 2), 4.9),
    ]


def test_to_mart_fails_on_unknown_product_or_unit():
    with pytest.raises(SourceFormatError, match="QUEROSENE"):
        transform.to_mart([parsed([row(SUNDAY, "QUEROSENE", "R$/l", 5.0)])])
    with pytest.raises(SourceFormatError, match="unidades"):
        transform.to_mart([parsed([row(SUNDAY, "GASOLINA COMUM", "R$/m3", 6.0)])])


def test_to_mart_fails_on_repeated_week_or_absurd_price():
    repeated = [parsed([row(SUNDAY, "GASOLINA COMUM", "R$/l", 6.0)])] * 2
    with pytest.raises((SchemaError, SchemaErrors)):
        transform.to_mart(repeated)
    with pytest.raises((SchemaError, SchemaErrors)):
        transform.to_mart([parsed([row(SUNDAY, "GASOLINA COMUM", "R$/l", 650.0)])])


def test_compare_with_stations_warns_only_above_limit():
    official = pl.DataFrame(
        {
            "product_id": ["gasolina", "gasolina", "etanol"],
            "week_start": [date(2026, 10, 19), date(2026, 10, 26), date(2026, 10, 26)],
            "avg_price": [6.0, 6.0, 4.0],
        }
    )
    ours = official.with_columns(
        avg_price=pl.when(pl.col("product_id") == "etanol").then(4.2).otherwise(6.03)
    )
    warnings = transform.compare_with_stations(official, ours, max_mean_diff_pct=2.0)
    assert warnings == [
        "etanol: média dos postos difere 5,00% da média oficial da ANP em 1 semanas (limite 2%)"
    ]
