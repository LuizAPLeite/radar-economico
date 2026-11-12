"""Job de análise: correlações do preço na bomba com o câmbio e o petróleo, e
a previsão de preços. Lê só a mart (o que os outros jobs gravaram) e grava
os resultados também na mart."""

import logging
from dataclasses import dataclass
from datetime import datetime

import polars as pl
import psycopg

from radar_pipeline import runs
from radar_pipeline.analysis import correlation, forecast
from radar_pipeline.analysis.regimes import CURRENT, REGIMES
from radar_pipeline.pg import copy_frame

log = logging.getLogger(__name__)

NAME = "analysis"

# Jobs cujos dados a análise usa: ela só roda de novo quando um deles
# carregou uma coleta nova.
INPUT_JOBS = ("bcb", "fred", "anp_serie")

# Produtos analisados (série oficial da ANP): o diesel comum tem histórico
# desde 2004; o S10, desde 2013.
PRODUCTS = ("gasolina", "etanol", "diesel", "diesel_s10")

_TABLES = (
    "mart.fuel_forecasts",
    "mart.forecast_backtests",
    "mart.analysis_rolling_correlations",
    "mart.analysis_lag_correlations",
    "mart.analysis_regimes",
)


@dataclass(frozen=True)
class Summary:
    source_run_id: int
    source_collected_at: datetime
    lag_rows: int
    rolling_rows: int
    forecast_rows: int
    backtest_rows: int
    warnings: tuple[str, ...] = ()

    @property
    def rows_written(self) -> int:
        return self.lag_rows + self.rolling_rows + self.forecast_rows + self.backtest_rows


def run(conn: psycopg.Connection, *, full_refresh: bool = False) -> Summary | None:
    inputs = conn.execute(
        """
        SELECT max(source_run_id), max(source_collected_at)
          FROM mart.pipeline_runs
         WHERE status = 'success' AND job = ANY(%s)
        """,
        (list(INPUT_JOBS),),
    ).fetchone()
    newest, collected_at = inputs
    if newest is None:
        log.warning("nenhum dos jobs de origem (%s) rodou ainda", ", ".join(INPUT_JOBS))
        return None
    if not full_refresh and newest <= (runs.last_source_run(conn, NAME) or 0):
        log.info("nada novo nos dados de origem desde a coleta %d", newest)
        return None

    prices = _prices(conn)
    daily = _daily_indicators(conn)
    missing = {"usd_brl", "brent"} - set(daily["indicator_id"].unique())
    if prices.is_empty() or missing:
        log.warning("faltam dados para a análise (série da ANP ou %s)", sorted(missing))
        return None
    drivers = correlation.weekly_drivers(daily)

    lag_frames, rolling_frames, forecast_frames, backtest_frames, warnings = [], [], [], [], []
    for product in PRODUCTS:
        series = prices.filter(pl.col("product_id") == product)
        if series.is_empty():
            warnings.append(f"{product}: sem série oficial da ANP para analisar")
            continue
        frame = correlation.panel(series, drivers)
        tag = pl.lit(product).alias("product_id")

        lags = correlation.lag_correlations(frame, REGIMES)
        lag_frames.append(lags.with_columns(tag))
        best = correlation.best_lags(lags, CURRENT.id)
        rolling_frames.append(correlation.rolling_correlations(frame, best).with_columns(tag))

        result = forecast.run(frame)
        if result is None:
            warnings.append(f"{product}: série curta demais para prever")
            continue
        forecast_frames.append(
            result.forecasts.with_columns(
                tag, model=pl.lit(result.model), base_week=pl.lit(result.base_week)
            )
        )
        backtest_frames.append(result.backtests.with_columns(tag))
        _log_product(product, best, result)

    for warning in warnings:
        log.warning("aviso: %s", warning)

    lag = _concat(lag_frames)
    rolling = _concat(rolling_frames)
    forecasts = _concat(forecast_frames)
    backtests = _concat(backtest_frames)
    _write(conn, lag, rolling, forecasts, backtests)

    return Summary(
        source_run_id=newest,
        source_collected_at=collected_at,
        lag_rows=lag.height,
        rolling_rows=rolling.height,
        forecast_rows=forecasts.height,
        backtest_rows=backtests.height,
        warnings=tuple(warnings),
    )


def _prices(conn: psycopg.Connection) -> pl.DataFrame:
    rows = conn.execute(
        """
        SELECT product_id, week_start, avg_price::float8
          FROM mart.fuel_prices_anp_weekly
         WHERE product_id = ANY(%s)
        """,
        (list(PRODUCTS),),
    ).fetchall()
    return pl.DataFrame(
        rows,
        schema={"product_id": pl.String, "week_start": pl.Date, "avg_price": pl.Float64},
        orient="row",
    )


def _daily_indicators(conn: psycopg.Connection) -> pl.DataFrame:
    rows = conn.execute(
        """
        SELECT indicator_id, ref_date, value::float8
          FROM mart.indicator_values
         WHERE indicator_id IN ('usd_brl', 'brent')
        """
    ).fetchall()
    return pl.DataFrame(
        rows,
        schema={"indicator_id": pl.String, "ref_date": pl.Date, "value": pl.Float64},
        orient="row",
    )


def _concat(frames: list[pl.DataFrame]) -> pl.DataFrame:
    return pl.concat(frames) if frames else pl.DataFrame()


def _write(
    conn: psycopg.Connection,
    lag: pl.DataFrame,
    rolling: pl.DataFrame,
    forecasts: pl.DataFrame,
    backtests: pl.DataFrame,
) -> None:
    """Substitui todos os resultados numa transação: a API nunca vê uma
    análise pela metade (MVCC, como nas outras cargas)."""
    regimes = pl.DataFrame(
        [(r.id, r.name, r.starts_on, r.ends_on, r.description) for r in REGIMES],
        schema={
            "id": pl.String,
            "name": pl.String,
            "starts_on": pl.Date,
            "ends_on": pl.Date,
            "description": pl.String,
        },
        orient="row",
    )
    columns = {
        "analysis_lag_correlations": (
            lag,
            ["product_id", "driver", "regime_id", "lag_weeks", "correlation", "n_weeks"],
        ),
        "analysis_rolling_correlations": (
            rolling,
            ["product_id", "driver", "week_start", "lag_weeks", "correlation"],
        ),
        "fuel_forecasts": (
            forecasts,
            [
                "product_id",
                "week_start",
                "model",
                "base_week",
                "horizon_weeks",
                "predicted",
                "lower_80",
                "upper_80",
            ],
        ),
        "forecast_backtests": (
            backtests,
            ["product_id", "model", "horizon_weeks", "mae", "mape", "n_forecasts"],
        ),
    }
    with conn.transaction(), conn.cursor() as cur:
        # A ordem respeita as chaves estrangeiras: quem referencia sai antes.
        for table in _TABLES:
            cur.execute(f"DELETE FROM {table}")  # nomes fixos, não vêm de fora
        copy_frame(cur, ("mart", "analysis_regimes"), regimes)
        for table, (frame, cols) in columns.items():
            if not frame.is_empty():
                copy_frame(cur, ("mart", table), frame.select(cols))
        cur.execute(f"ANALYZE {', '.join(_TABLES)}")


def _log_product(product: str, best: dict[str, int], result: forecast.Result) -> None:
    mae = {(m, h): v for m, h, v in result.backtests.select("model", "horizon_weeks", "mae").rows()}
    first = result.forecasts.row(0, named=True) if result.forecasts.height else None
    log.info(
        "%s: defasagem do Brent em R$ no período atual: %s semanas; modelo publicado: %s "
        "(erro médio em 4 semanas: naive R$ %.3f, brent R$ %s); próxima semana: %s",
        product,
        best.get("brent_brl"),
        result.model,
        mae.get(("naive", 4), float("nan")),
        f"{mae[('brent', 4)]:.3f}" if ("brent", 4) in mae else "-",
        f"R$ {first['predicted']:.3f} ({first['lower_80']:.3f} a {first['upper_80']:.3f})"
        if first
        else "-",
    )
