"""Testes da previsão e do backtest (sem banco), com séries sintéticas."""

from datetime import date, timedelta

import numpy as np
import polars as pl

from radar_pipeline.analysis import forecast

START = date(2018, 1, 1)  # uma segunda-feira
N_WEEKS = 320


def frame(price: np.ndarray, brent_brl: np.ndarray) -> pl.DataFrame:
    return pl.DataFrame(
        {
            "week_start": [START + timedelta(weeks=i) for i in range(len(price))],
            "avg_price": price,
            "brent_brl": brent_brl,
        }
    )


def follows_brent(seed: int = 3, lag: int = 3) -> pl.DataFrame:
    """Preço que repassa metade da variação do Brent em R$ com `lag` semanas
    de atraso: o modelo do Brent tem informação que o naive não tem."""
    rng = np.random.default_rng(seed)
    brent = 400 * np.exp(np.cumsum(rng.normal(0, 0.04, N_WEEKS)))
    price = np.full(N_WEEKS, 6.0)
    price[lag:] = 6.0 * (brent[:-lag] / 400) ** 0.5
    price *= np.exp(rng.normal(0, 0.001, N_WEEKS))
    return frame(price, brent)


def unrelated(seed: int = 5) -> pl.DataFrame:
    rng = np.random.default_rng(seed)
    brent = 400 * np.exp(np.cumsum(rng.normal(0, 0.04, N_WEEKS)))
    price = 6.0 * np.exp(np.cumsum(rng.normal(0, 0.01, N_WEEKS)))
    return frame(price, brent)


def test_features_use_only_past_weeks():
    brent = np.exp(np.arange(12.0))  # log do Brent = índice da semana
    x = forecast.features(np.ones(12), brent)
    assert np.isnan(x[8, 1])
    # Semana 10: variações de 5→9 e de 1→5, as duas de 4 semanas.
    assert x[10].tolist() == [1.0, 4.0, 4.0]


def test_brent_model_wins_when_price_follows_brent():
    result = forecast.run(follows_brent())
    assert result is not None
    assert result.model == "brent"

    mae = {(m, h): v for m, h, v in result.backtests.select("model", "horizon_weeks", "mae").rows()}
    assert mae["brent", 2] < mae["naive", 2] * 0.8


def test_naive_is_kept_when_brent_has_no_information():
    result = forecast.run(unrelated())
    assert result.model == "naive"
    # O naive prevê o último preço, em todos os horizontes.
    last = unrelated()["avg_price"][-1]
    assert result.forecasts["predicted"].to_list() == [round(last, 3)] * 8


def test_forecast_weeks_intervals_and_backtest_shape():
    result = forecast.run(follows_brent())
    f = result.forecasts
    assert result.base_week == START + timedelta(weeks=N_WEEKS - 1)
    assert f["horizon_weeks"].to_list() == list(forecast.HORIZONS)
    assert f["week_start"].to_list() == [
        result.base_week + timedelta(weeks=h) for h in forecast.HORIZONS
    ]
    assert (f["lower_80"] <= f["predicted"]).all()
    assert (f["predicted"] <= f["upper_80"]).all()
    # Dois modelos × 8 horizontes, cada um com as 104 origens do backtest.
    assert result.backtests.height == 16
    assert set(result.backtests["n_forecasts"]) == {forecast.BACKTEST_WEEKS}


def test_missing_recent_brent_falls_back_to_naive():
    data = follows_brent()
    # O FRED atrasou: sem Brent nas últimas 10 semanas.
    recent = pl.col("week_start") >= START + timedelta(weeks=N_WEEKS - 10)
    data = data.with_columns(brent_brl=pl.when(recent).then(None).otherwise(pl.col("brent_brl")))
    result = forecast.run(data)
    assert result.model == "naive"
    assert result.forecasts.height == 8


def test_missing_weeks_are_skipped_not_invented():
    data = follows_brent()
    gap = pl.col("week_start").is_between(
        START + timedelta(weeks=250), START + timedelta(weeks=259)
    )
    data = data.with_columns(avg_price=pl.when(gap).then(None).otherwise(pl.col("avg_price")))
    result = forecast.run(data)
    # Origens e alvos que caem no buraco ficam fora do backtest.
    assert result.backtests["n_forecasts"].max() < forecast.BACKTEST_WEEKS


def test_short_series_has_no_forecast():
    assert forecast.run(follows_brent().head(100)) is None
