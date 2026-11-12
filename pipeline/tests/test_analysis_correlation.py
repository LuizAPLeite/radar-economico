"""Testes das correlações (sem banco), com séries sintéticas de relação conhecida."""

from datetime import date, timedelta

import numpy as np
import polars as pl
import pytest

from radar_pipeline.analysis import correlation
from radar_pipeline.analysis.regimes import Regime

START = date(2020, 1, 6)  # uma segunda-feira
N_WEEKS = 300
TRUE_LAG = 2


def synthetic(seed: int = 7) -> tuple[pl.DataFrame, pl.DataFrame]:
    """Fatores em passeio aleatório; o preço segue o Brent em R$ com TRUE_LAG
    semanas de atraso, mais ruído. O dólar não tem relação com o preço."""
    rng = np.random.default_rng(seed)
    weeks = [START + timedelta(weeks=i) for i in range(N_WEEKS)]
    brent_brl = 300 * np.exp(np.cumsum(rng.normal(0, 0.03, N_WEEKS)))
    usd = 5 * np.exp(np.cumsum(rng.normal(0, 0.01, N_WEEKS)))
    price = np.empty(N_WEEKS)
    price[:TRUE_LAG] = 5.0
    price[TRUE_LAG:] = 5.0 * (brent_brl[:-TRUE_LAG] / 300) ** 0.5
    price *= np.exp(rng.normal(0, 0.002, N_WEEKS))
    drivers = pl.DataFrame(
        {
            "week_start": weeks,
            "usd_brl": usd,
            "brent_usd": brent_brl / usd,
            "brent_brl": brent_brl,
        }
    )
    prices = pl.DataFrame({"week_start": weeks, "avg_price": price})
    return prices, drivers


ALL_TIME = (Regime("tudo", "Tudo", date(2000, 1, 1), None, ""),)


def test_weekly_drivers_averages_by_monday_week_and_multiplies_same_day():
    daily = pl.DataFrame(
        {
            "indicator_id": ["usd_brl", "usd_brl", "brent", "brent", "usd_brl"],
            # seg e ter da mesma semana; o Brent de quarta não tem dólar no dia
            "ref_date": [date(2026, 11, 2), date(2026, 11, 3)] * 2 + [date(2026, 11, 9)],
            "value": [5.0, 6.0, 100.0, 110.0, 5.5],
        }
    )
    weekly = correlation.weekly_drivers(daily)
    assert weekly.rows() == [
        (date(2026, 11, 2), 5.5, 105.0, (500 + 660) / 2),
        (date(2026, 11, 9), 5.5, None, None),
    ]


def test_panel_fills_missing_weeks_with_nulls_without_interpolating():
    prices = pl.DataFrame(
        {
            "week_start": [START, START + timedelta(weeks=2)],  # falta a semana do meio
            "avg_price": [5.0, 6.0],
        }
    )
    drivers = pl.DataFrame(
        {"week_start": [START], "usd_brl": [5.0], "brent_usd": [80.0], "brent_brl": [400.0]}
    )
    frame = correlation.panel(prices, drivers)
    assert frame["week_start"].to_list() == [START + timedelta(weeks=i) for i in range(3)]
    assert frame["avg_price"].to_list() == [5.0, None, 6.0]


def test_best_lag_recovers_the_true_delay():
    prices, drivers = synthetic()
    frame = correlation.panel(prices, drivers)
    corr = correlation.lag_correlations(frame, ALL_TIME)

    assert correlation.best_lags(corr, "tudo")["brent_brl"] == TRUE_LAG
    at_lag = corr.filter((pl.col("driver") == "brent_brl") & (pl.col("lag_weeks") == TRUE_LAG)).row(
        0, named=True
    )
    assert at_lag["correlation"] > 0.9
    # Sem relação: perto de zero em qualquer defasagem.
    usd = corr.filter(pl.col("driver") == "usd_brl")["correlation"]
    assert usd.abs().max() < 0.3


def test_regimes_split_the_sample_and_short_ones_are_dropped():
    prices, drivers = synthetic()
    frame = correlation.panel(prices, drivers)
    regimes = (
        Regime("antes", "Antes", date(2000, 1, 1), START + timedelta(weeks=150), ""),
        Regime("depois", "Depois", START + timedelta(weeks=150), START + timedelta(weeks=180), ""),
    )
    corr = correlation.lag_correlations(frame, regimes)
    # "depois" tem só 30 semanas, menos que MIN_PAIRS.
    assert set(corr["regime_id"]) == {"antes"}
    antes = corr.filter((pl.col("driver") == "brent_brl") & (pl.col("lag_weeks") == 0))
    assert antes["n_weeks"].item() == 150 - correlation.CHANGE_WEEKS


def test_rolling_correlation_windows():
    prices, drivers = synthetic()
    frame = correlation.panel(prices, drivers)
    rolling = correlation.rolling_correlations(frame, {"brent_brl": TRUE_LAG})

    # A primeira janela com pares suficientes, e a última semana do painel.
    assert rolling["week_start"].max() == frame["week_start"].max()
    assert rolling["lag_weeks"].unique().to_list() == [TRUE_LAG]
    assert rolling["correlation"].min() > 0.8


def test_constant_price_has_no_correlation():
    prices, drivers = synthetic()
    frozen = prices.with_columns(avg_price=pl.lit(5.0))
    frame = correlation.panel(frozen, drivers)
    assert correlation.lag_correlations(frame, ALL_TIME).is_empty()
    assert correlation.rolling_correlations(frame, {"brent_brl": 0}).is_empty()


@pytest.mark.parametrize("lag", [0, 5])
def test_lag_shifts_the_driver_back_in_time(lag):
    prices, drivers = synthetic()
    frame = correlation.panel(prices, drivers)
    corr = correlation.lag_correlations(frame, ALL_TIME)
    n = corr.filter((pl.col("driver") == "brent_brl") & (pl.col("lag_weeks") == lag))["n_weeks"]
    # Cada semana de defasagem custa um par no começo da série.
    assert n.item() == N_WEEKS - correlation.CHANGE_WEEKS - lag
