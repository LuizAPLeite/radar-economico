from datetime import date

import polars as pl
import pytest
from pandera.errors import SchemaErrors

from radar_pipeline.anp.transform import (
    SourceFormatError,
    municipal_monthly,
    parse_file,
    to_staging,
    weekly_averages,
)

# Cabeçalho idêntico ao da ANP, com o BOM do UTF-8 no começo.
HEADER = (
    "﻿Regiao - Sigla;Estado - Sigla;Municipio;Revenda;CNPJ da Revenda;Nome da Rua;"
    "Numero Rua;Complemento;Bairro;Cep;Produto;Data da Coleta;Valor de Venda;"
    "Valor de Compra;Unidade de Medida;Bandeira"
)


def row(
    uf="SP",
    municipio="GUARULHOS",
    cnpj="49.051.667/0001-02",
    produto="GASOLINA",
    data="07/10/2026",
    preco="6,79",
    unidade="R$ / litro",
    bandeira="VIBRA",
) -> str:
    # Mesmo layout das linhas reais: o CNPJ vem com um espaço na frente.
    return (
        f"SE;{uf};{municipio};AUTO POSTO X; {cnpj};RUA X;1;;CENTRO;07178-580;"
        f"{produto};{data};{preco};;{unidade};{bandeira}"
    )


def csv(*rows: str) -> bytes:
    return ("\r\n".join([HEADER, *rows]) + "\r\n").encode("utf-8")


MUNICIPALITIES = pl.DataFrame(
    {
        "ibge_code": [3518800, 3550308, 4317103, 2903201],
        "name": ["Guarulhos", "São Paulo", "Sant'Ana do Livramento", "Barreiras"],
        "uf": ["SP", "SP", "RS", "BA"],
    }
)


def staging_of(*rows: str):
    return to_staging([parse_file(csv(*rows), file_id=7)], MUNICIPALITIES)


# --- parse_file -----------------------------------------------------------------


def test_parse_file_types_the_columns():
    df = parse_file(csv(row()), file_id=7)
    assert df.row(0, named=True) == {
        "uf": "SP",
        "municipality": "GUARULHOS",
        "cnpj": "49051667000102",  # só dígitos, sem o espaço da fonte
        "product_anp": "GASOLINA",
        "collected_on": date(2026, 10, 7),
        "price": 6.79,  # vírgula decimal convertida
        "unit": "R$ / litro",
        "brand": "VIBRA",
        "source_file_id": 7,
    }


@pytest.mark.parametrize(
    "bad_row",
    [row(preco="seis reais"), row(data="2026-10-07"), row(data="31/02/2026")],
)
def test_parse_file_fails_loudly_on_unexpected_format(bad_row):
    with pytest.raises(pl.exceptions.PolarsError):
        parse_file(csv(bad_row), file_id=7)


# --- to_staging -----------------------------------------------------------------


def test_maps_products_and_matches_municipalities():
    result = staging_of(
        row(municipio="SAO PAULO", produto="DIESEL S10", preco="6,99"),  # sem acento na ANP
        row(uf="RS", municipio="SANTANA DO LIVRAMENTO", cnpj="11.111.111/0001-11"),  # exceção
        row(produto="GNV", unidade="R$ / m³", preco="4,59", cnpj="22.222.222/0001-22"),
    )
    assert result.frame.select("product", "ibge_code", "price").rows() == [
        ("diesel_s10", 3550308, 6.99),
        ("gasolina", 4317103, 6.79),
        ("gnv", 3518800, 4.59),
    ]
    assert result.duplicates_removed == 0


def test_unknown_product_fails_with_its_name():
    with pytest.raises(SourceFormatError, match="GASOLINA PREMIUM"):
        staging_of(row(produto="GASOLINA PREMIUM"))


def test_unexpected_unit_fails():
    with pytest.raises(SourceFormatError, match="unidades"):
        staging_of(row(unidade="R$ / galão"))


def test_unmatched_municipality_fails_listing_it():
    with pytest.raises(SourceFormatError, match="CIDADE INVENTADA"):
        staging_of(row(municipio="CIDADE INVENTADA"))


def test_identical_duplicates_are_removed():
    result = staging_of(row(), row())
    assert result.frame.height == 1
    assert result.duplicates_removed == 1


def test_same_station_and_day_with_different_prices_fails():
    with pytest.raises(SchemaErrors, match="uniqueness"):
        staging_of(row(preco="6,79"), row(preco="6,89"))


@pytest.mark.parametrize("price", ["0,00", "69,90", "1,50"])
def test_price_out_of_range_fails(price):
    with pytest.raises(SchemaErrors, match="price_within_product_range"):
        staging_of(row(preco=price))


# --- agregados ------------------------------------------------------------------


def test_weekly_averages_for_country_and_states():
    staging = staging_of(
        # semana de 05/10/2026 (segunda): dois postos em SP, um na BA
        row(data="06/10/2026", preco="6,00"),
        row(data="07/10/2026", preco="7,00", cnpj="22.222.222/0001-22"),
        row(
            uf="BA",
            municipio="BARREIRAS",
            data="08/10/2026",
            preco="8,00",
            cnpj="33.333.333/0001-33",
        ),
        # semana seguinte
        row(data="13/10/2026", preco="6,50"),
    ).frame

    weekly = weekly_averages(staging).frame

    assert weekly.rows() == [
        ("gasolina", "BA", date(2026, 10, 5), 8.0, 1),
        ("gasolina", "BR", date(2026, 10, 5), 7.0, 3),
        ("gasolina", "BR", date(2026, 10, 12), 6.5, 1),
        ("gasolina", "SP", date(2026, 10, 5), 6.5, 2),
        ("gasolina", "SP", date(2026, 10, 12), 6.5, 1),
    ]


def test_incomplete_week_is_dropped_everywhere():
    # Três semanas com 4 postos e uma com só 1: abaixo de metade da mediana (4).
    rows = []
    for k, day in enumerate(["05", "12", "19"]):
        for station in range(4):
            rows.append(row(data=f"{day}/10/2026", cnpj=f"1{k}.{station}11.111/0001-11"))
    rows.append(row(data="26/10/2026", cnpj="99.999.999/0001-99"))

    result = weekly_averages(staging_of(*rows).frame)

    assert result.incomplete_weeks == 1
    assert date(2026, 10, 26) not in result.frame["week_start"].to_list()  # nem BR, nem SP
    assert result.frame.filter(scope="BR").height == 3


def test_municipal_monthly_min_avg_max():
    staging = staging_of(
        row(data="06/10/2026", preco="6,00"),
        row(data="20/10/2026", preco="7,00"),
        row(data="20/10/2026", preco="6,40", cnpj="22.222.222/0001-22"),
        row(data="03/11/2026", preco="6,90"),
    ).frame

    monthly = municipal_monthly(staging)

    assert monthly.rows() == [
        ("gasolina", date(2026, 10, 1), 3518800, 6.467, 6.0, 7.0, 2),
        ("gasolina", date(2026, 11, 1), 3518800, 6.9, 6.9, 6.9, 1),
    ]
