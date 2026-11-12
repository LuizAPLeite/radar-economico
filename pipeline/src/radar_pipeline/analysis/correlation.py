"""Correlação entre a variação do preço na bomba e a variação do câmbio e do
petróleo semanas antes. Funções puras: recebem e devolvem DataFrames."""

import numpy as np
import polars as pl

from radar_pipeline.analysis.regimes import Regime

# Os fatores comparados com o preço na bomba.
DRIVERS = ("usd_brl", "brent_usd", "brent_brl")

# Variação em 4 semanas (≈ 1 mês), e não semana a semana: os reajustes vêm em
# degraus e a média semanal tem ruído de amostra, que esconderia a relação.
CHANGE_WEEKS = 4

# Defasagens testadas: o efeito de uma alta do petróleo hoje na bomba daqui a
# 0, 1, ..., 12 semanas.
LAGS = range(13)

# Menos pares que isso e a correlação é ruído demais para publicar.
MIN_PAIRS = 52

# Janela da correlação móvel (1 ano) e o mínimo de pares dentro dela.
ROLLING_WEEKS = 52
ROLLING_MIN_PAIRS = 40


def weekly_drivers(daily: pl.DataFrame) -> pl.DataFrame:
    """Médias semanais (semana começando na segunda) do dólar, do Brent em US$
    e do Brent em R$ (Brent × dólar do mesmo dia; só os dias com os dois).

    daily: indicator_id ("usd_brl", "brent"), ref_date, value.
    """
    usd = daily.filter(pl.col("indicator_id") == "usd_brl").select("ref_date", usd=pl.col("value"))
    brent = daily.filter(pl.col("indicator_id") == "brent").select(
        "ref_date", brent=pl.col("value")
    )
    week = pl.col("ref_date").dt.truncate("1w").alias("week_start")
    usd_w = usd.group_by(week).agg(usd_brl=pl.col("usd").mean())
    brent_w = brent.group_by(week).agg(brent_usd=pl.col("brent").mean())
    brl_w = (
        usd.join(brent, on="ref_date")
        .group_by(week)
        .agg(brent_brl=(pl.col("brent") * pl.col("usd")).mean())
    )
    return (
        usd_w.join(brent_w, on="week_start", how="full", coalesce=True)
        .join(brl_w, on="week_start", how="full", coalesce=True)
        .sort("week_start")
    )


def panel(prices: pl.DataFrame, drivers: pl.DataFrame) -> pl.DataFrame:
    """Uma linha por semana do calendário (sem buracos), com o preço, os
    fatores e as variações logarítmicas em CHANGE_WEEKS semanas.

    prices: week_start, avg_price (um produto). Semanas sem pesquisa da ANP
    (houve algumas, ex.: ago-out/2020) ficam com nulo, e as variações que
    dependem delas também: nada é interpolado.
    """
    first, last = prices["week_start"].min(), prices["week_start"].max()
    calendar = pl.DataFrame({"week_start": pl.date_range(first, last, interval="1w", eager=True)})
    frame = calendar.join(prices.select("week_start", "avg_price"), on="week_start", how="left")
    frame = frame.join(drivers, on="week_start", how="left").sort("week_start")
    return frame.with_columns(
        _log_change("avg_price").alias("price_chg"),
        *(_log_change(d).alias(f"{d}_chg") for d in DRIVERS),
    )


def _log_change(column: str) -> pl.Expr:
    return pl.col(column).log() - pl.col(column).log().shift(CHANGE_WEEKS)


def lag_correlations(frame: pl.DataFrame, regimes: tuple[Regime, ...]) -> pl.DataFrame:
    """Correlação entre a variação do preço na semana t e a do fator na semana
    t - lag, para cada fator, período e defasagem.

    frame: saída de panel(). Uma semana entra no período pela data do preço
    (o fator defasado pode vir de antes do início do período). Combinações com
    menos de MIN_PAIRS pares ficam de fora.
    """
    rows = []
    for driver in DRIVERS:
        for lag in LAGS:
            pairs = frame.select(
                "week_start", "price_chg", pl.col(f"{driver}_chg").shift(lag).alias("x")
            ).drop_nulls()
            for regime in regimes:
                in_regime = pairs.filter(_in_regime(regime))
                if in_regime.height < MIN_PAIRS:
                    continue
                corr = _pearson(in_regime["price_chg"], in_regime["x"])
                if corr is None:
                    continue
                rows.append(
                    {
                        "driver": driver,
                        "regime_id": regime.id,
                        "lag_weeks": lag,
                        "correlation": corr,
                        "n_weeks": in_regime.height,
                    }
                )
    return pl.DataFrame(
        rows,
        schema={
            "driver": pl.String,
            "regime_id": pl.String,
            "lag_weeks": pl.Int64,
            "correlation": pl.Float64,
            "n_weeks": pl.Int64,
        },
    )


def _in_regime(regime: Regime) -> pl.Expr:
    expr = pl.col("week_start") >= regime.starts_on
    if regime.ends_on is not None:
        expr &= pl.col("week_start") < regime.ends_on
    return expr


def best_lags(correlations: pl.DataFrame, regime_id: str) -> dict[str, int]:
    """Para cada fator, a defasagem de maior correlação num período."""
    best = (
        correlations.filter(pl.col("regime_id") == regime_id)
        .sort("correlation", descending=True)
        .group_by("driver", maintain_order=True)
        .first()
    )
    return dict(best.select("driver", "lag_weeks").iter_rows())


def rolling_correlations(frame: pl.DataFrame, lags: dict[str, int]) -> pl.DataFrame:
    """Correlação em janela móvel de ROLLING_WEEKS semanas do calendário, para
    cada fator na defasagem dada. week_start é a última semana da janela;
    janelas com menos de ROLLING_MIN_PAIRS pares ficam de fora."""
    rows = []
    weeks = frame["week_start"].to_list()
    y_all = frame["price_chg"].to_numpy()
    for driver, lag in lags.items():
        x_all = frame[f"{driver}_chg"].shift(lag).to_numpy()
        for end in range(ROLLING_WEEKS - 1, len(weeks)):
            y = y_all[end - ROLLING_WEEKS + 1 : end + 1]
            x = x_all[end - ROLLING_WEEKS + 1 : end + 1]
            ok = ~(np.isnan(y) | np.isnan(x))
            if ok.sum() < ROLLING_MIN_PAIRS:
                continue
            corr = _pearson(pl.Series(y[ok]), pl.Series(x[ok]))
            if corr is None:
                continue
            rows.append(
                {"driver": driver, "week_start": weeks[end], "lag_weeks": lag, "correlation": corr}
            )
    return pl.DataFrame(
        rows,
        schema={
            "driver": pl.String,
            "week_start": pl.Date,
            "lag_weeks": pl.Int64,
            "correlation": pl.Float64,
        },
    )


def _pearson(y: pl.Series, x: pl.Series) -> float | None:
    """Correlação de Pearson; None se uma das séries não varia (ex.: preço
    congelado o ano inteiro), caso em que a correlação não existe."""
    y_np, x_np = y.to_numpy().astype(float), x.to_numpy().astype(float)
    if y_np.std() == 0 or x_np.std() == 0:
        return None
    return float(np.corrcoef(y_np, x_np)[0, 1])
