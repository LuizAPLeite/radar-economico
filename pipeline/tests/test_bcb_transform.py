import json
from datetime import date

import polars as pl
import pytest
from pandera.errors import SchemaErrors

from radar_pipeline.bcb.transform import (
    RAW_SCHEMA,
    explode_responses,
    to_mart_values,
    to_staging,
)


def raw(*rows: tuple[int, int, object]) -> pl.DataFrame:
    """Linhas como as de raw.bcb_sgs_responses; o payload vira texto JSON."""
    return pl.DataFrame(
        [(code, status, json.dumps(payload)) for code, status, payload in rows],
        schema=RAW_SCHEMA,
        orient="row",
    )


def obs(data: str, valor: str | None) -> dict[str, str | None]:
    return {"data": data, "valor": valor}


def observations(*rows: tuple[int, str, str | None]) -> pl.DataFrame:
    return pl.DataFrame(
        rows,
        schema={"series_code": pl.Int64, "data": pl.String, "valor": pl.String},
        orient="row",
    )


def staging(*rows: tuple[int, date, float]) -> pl.DataFrame:
    return pl.DataFrame(
        [(code, ref_date, value, 1) for code, ref_date, value in rows],
        schema={
            "series_code": pl.Int64,
            "ref_date": pl.Date,
            "value": pl.Float64,
            "run_id": pl.Int64,
        },
        orient="row",
    )


# --- explode_responses -------------------------------------------------------


def test_explode_gives_one_row_per_observation_and_ignores_404():
    df = explode_responses(
        raw(
            (1, 200, [obs("05/11/2026", "5.4621"), obs("06/11/2026", "5.5013")]),
            (433, 200, [obs("01/09/2026", "0.82")]),
            (1, 404, {"erro": {"statusCode": 404, "detail": "Value(s) not found"}}),
        )
    )
    assert df.rows() == [
        (1, "05/11/2026", "5.4621"),
        (1, "06/11/2026", "5.5013"),
        (433, "01/09/2026", "0.82"),
    ]


def test_explode_empty_array_has_no_rows():
    assert explode_responses(raw((1, 200, []))).height == 0


# --- to_staging --------------------------------------------------------------


def test_to_staging_types_and_sorts():
    result = to_staging(
        observations(
            (433, "01/08/2026", "-0.32"),
            (1, "06/11/2026", "5.5013"),
            (433, "01/07/2026", "0.07"),
        ),
        run_id=42,
    )
    assert result.frame.rows() == [
        (1, date(2026, 11, 6), 5.5013, 42),
        (433, date(2026, 7, 1), 0.07, 42),
        (433, date(2026, 8, 1), -0.32, 42),
    ]
    assert result.without_value == 0


def test_to_staging_drops_and_counts_observations_without_value():
    result = to_staging(
        observations(
            (1, "05/11/2026", "5.4621"),
            (1, "06/11/2026", ""),
            (1, "07/11/2026", "   "),
            (1, "08/11/2026", None),
        ),
        run_id=1,
    )
    assert result.frame.height == 1
    assert result.without_value == 3


@pytest.mark.parametrize(
    ("data", "valor"),
    [
        ("2026-11-06", "5.5013"),  # data em outro formato
        ("31/02/2026", "5.5013"),  # data que não existe
        ("06/11/2026", "5,5013"),  # vírgula decimal
        ("06/11/2026", "abc"),
    ],
)
def test_to_staging_fails_loudly_on_unexpected_format(data, valor):
    with pytest.raises(pl.exceptions.PolarsError):
        to_staging(observations((1, data, valor)), run_id=1)


def test_to_staging_rejects_duplicate_dates():
    with pytest.raises(SchemaErrors, match="uniqueness"):
        to_staging(
            observations((1, "06/11/2026", "5.50"), (1, "06/11/2026", "5.51")),
            run_id=1,
        )


# --- to_mart_values ----------------------------------------------------------


def test_to_mart_maps_series_to_indicators():
    result = to_mart_values(
        staging(
            (432, date(2026, 11, 6), 13.75),
            (433, date(2026, 9, 1), 0.82),
            (1, date(2026, 11, 6), 5.5013),
            (9999, date(2026, 11, 6), 1.0),  # série fora do catálogo
        ),
        as_of=date(2026, 11, 7),
    )
    assert result.frame.rows() == [
        ("ipca", date(2026, 9, 1), 0.82),
        ("selic", date(2026, 11, 6), 13.75),
        ("usd_brl", date(2026, 11, 6), 5.5013),
    ]
    assert result.unknown_series == [9999]


def test_to_mart_drops_dates_after_as_of():
    result = to_mart_values(
        staging(
            (432, date(2026, 11, 7), 13.75),
            (432, date(2026, 11, 8), 13.75),
            (432, date(2026, 12, 9), 13.75),
        ),
        as_of=date(2026, 11, 7),
    )
    assert result.frame["ref_date"].to_list() == [date(2026, 11, 7)]
    assert result.future_dropped == 2


@pytest.mark.parametrize(
    ("series_code", "value"),
    [
        (433, 150.0),  # IPCA acima de 100% ao mês
        (433, -5.0),  # IPCA no limite inferior (exclusivo)
        (1, 0.0),  # dólar precisa ser maior que zero
        (1, 5501.3),  # erro de escala
        (432, -1.0),  # Selic negativa
    ],
)
def test_to_mart_rejects_values_out_of_range(series_code, value):
    with pytest.raises(SchemaErrors, match="value_within_indicator_range"):
        to_mart_values(staging((series_code, date(2026, 1, 1), value)), as_of=date(2026, 11, 7))


def test_to_mart_accepts_range_limits():
    result = to_mart_values(
        staging((433, date(2026, 1, 1), 100.0), (433, date(2026, 2, 1), -4.99)),
        as_of=date(2026, 11, 7),
    )
    assert result.frame.height == 2
