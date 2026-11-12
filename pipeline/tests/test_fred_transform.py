"""Testes das transformações do FRED (sem banco)."""

from datetime import date

import pytest
from pandera.errors import SchemaError, SchemaErrors

from radar_pipeline.anp.transform import SourceFormatError
from radar_pipeline.fred import transform


def csv(*lines: str) -> bytes:
    return ("observation_date,DCOILBRENTEU\n" + "\n".join(lines) + "\n").encode()


def test_parse_skips_days_without_quote():
    frame = transform.parse_csv(
        csv("2026-11-02,107.39", "2026-11-03,", "2026-11-04,.", "2026-11-05,106.5"),
        transform.BRENT,
    )
    assert frame.rows() == [
        ("brent", date(2026, 11, 2), 107.39),
        ("brent", date(2026, 11, 5), 106.5),
    ]


def test_parse_fails_on_other_columns():
    with pytest.raises(SourceFormatError, match="DCOILWTICO"):
        transform.parse_csv(b"observation_date,DCOILWTICO\n2026-11-02,1\n", transform.BRENT)


@pytest.mark.parametrize(
    "line",
    [
        "2026-11-02,abc",  # número inválido
        "02/11/2026,107.3",  # outro formato de data
    ],
)
def test_parse_fails_on_malformed_values(line):
    with pytest.raises(Exception):  # noqa: B017 (polars levanta tipos diferentes)
        transform.parse_csv(csv(line), transform.BRENT)


def test_contract_rejects_absurd_values_and_repeated_dates():
    with pytest.raises((SchemaError, SchemaErrors)):
        transform.parse_csv(csv("2026-11-02,10739"), transform.BRENT)  # escala errada
    with pytest.raises((SchemaError, SchemaErrors)):
        transform.parse_csv(csv("2026-11-02,0"), transform.BRENT)
    with pytest.raises((SchemaError, SchemaErrors)):
        transform.parse_csv(csv("2026-11-02,107.3", "2026-11-02,107.4"), transform.BRENT)
