import polars as pl

from radar_pipeline import LAYERS


def test_layers_in_order():
    assert LAYERS == ("raw", "staging", "mart")


def test_polars_available():
    df = pl.DataFrame({"value": [1, 2, 3]})
    assert df["value"].sum() == 6
