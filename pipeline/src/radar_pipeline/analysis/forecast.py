"""Previsão do preço médio semanal na bomba para as próximas semanas, com
teste honesto contra o passado (backtest). Funções puras, com numpy.

Dois modelos:
- naive: "o preço fica igual". Difícil de bater num preço que muda em
  degraus, e é a régua do outro;
- brent: regressão da variação futura do preço sobre as variações recentes
  do Brent em reais (o que move a bomba desde a paridade internacional).

Publica-se a previsão do modelo com menor erro no backtest, com um
intervalo de 80% tirado dos erros que ele cometeu lá.
"""

from dataclasses import dataclass
from datetime import date, timedelta

import numpy as np
import polars as pl

HORIZONS = range(1, 9)  # semanas à frente

# A regressão aprende só com os últimos 3 anos: a relação muda com a
# política de preços, e dados antigos demais atrapalham.
TRAIN_WEEKS = 156
MIN_TRAIN = 52

# Origens do backtest: as semanas dos últimos 2 anos.
BACKTEST_WEEKS = 104

MODELS = ("naive", "brent")

# Intervalo de 80%: entre os percentis 10 e 90 dos erros do backtest.
LOWER_Q, UPPER_Q = 0.10, 0.90


@dataclass(frozen=True)
class Result:
    model: str  # o modelo publicado
    base_week: date
    forecasts: pl.DataFrame  # week_start, horizon_weeks, predicted, lower_80, upper_80
    backtests: pl.DataFrame  # model, horizon_weeks, mae, mape, n_forecasts


def features(price: np.ndarray, brent_brl: np.ndarray) -> np.ndarray:
    """Matriz de variáveis para cada semana i (linhas): constante e as
    variações log do Brent em R$ nas janelas [i-5, i-1] e [i-9, i-5].

    Usa até a semana ANTERIOR (i-1): na semana mais recente o FRED ainda
    pode não ter publicado o Brent. Linhas sem dados ficam com NaN.
    """
    log_b = np.log(brent_brl)
    n = len(price)
    x = np.full((n, 3), np.nan)
    x[:, 0] = 1.0
    x[9:, 1] = log_b[8:-1] - log_b[4:-5]
    x[9:, 2] = log_b[4:-5] - log_b[: n - 9]
    return x


def _fit_predict(x: np.ndarray, log_p: np.ndarray, origin: int, h: int) -> float | None:
    """Ajusta, com o que se sabia na semana `origin`, a regressão da variação
    em h semanas e prevê a partir de `origin`. None sem dados suficientes."""
    first = max(0, origin - TRAIN_WEEKS)
    j = np.arange(first, origin - h + 1)  # o alvo j+h já era conhecido em origin
    if len(j) == 0:
        return None
    y = log_p[j + h] - log_p[j]
    xj = x[j]
    ok = ~(np.isnan(y) | np.isnan(xj).any(axis=1))
    if ok.sum() < MIN_TRAIN or np.isnan(x[origin]).any() or np.isnan(log_p[origin]):
        return None
    coef, *_ = np.linalg.lstsq(xj[ok], y[ok], rcond=None)
    return float(log_p[origin] + x[origin] @ coef)


def _predict(model: str, x: np.ndarray, log_p: np.ndarray, origin: int, h: int) -> float | None:
    if np.isnan(log_p[origin]):
        return None
    if model == "naive":
        return float(log_p[origin])
    return _fit_predict(x, log_p, origin, h)


def run(frame: pl.DataFrame) -> Result | None:
    """Backtest dos modelos e previsão a partir da última semana com preço.

    frame: uma linha por semana do calendário, sem buracos (ver
    correlation.panel), com avg_price e brent_brl (nulos onde faltam).
    None se a série for curta demais.
    """
    weeks = frame["week_start"].to_list()
    price = frame["avg_price"].fill_null(np.nan).to_numpy().astype(float)
    brent = frame["brent_brl"].fill_null(np.nan).to_numpy().astype(float)
    log_p = np.log(price)
    x = features(price, brent)

    observed = np.flatnonzero(~np.isnan(log_p))
    if len(observed) < MIN_TRAIN + BACKTEST_WEEKS:
        return None
    base = int(observed[-1])

    backtests, errors = [], {}
    for model in MODELS:
        for h in HORIZONS:
            log_errors, abs_errors, pct_errors = [], [], []
            for origin in range(base - h - BACKTEST_WEEKS + 1, base - h + 1):
                predicted = _predict(model, x, log_p, origin, h)
                actual = log_p[origin + h]
                if predicted is None or np.isnan(actual):
                    continue
                log_errors.append(actual - predicted)
                abs_errors.append(abs(np.exp(actual) - np.exp(predicted)))
                pct_errors.append(abs(np.exp(actual) / np.exp(predicted) - 1) * 100)
            if not abs_errors:
                continue
            errors[model, h] = np.array(log_errors)
            backtests.append(
                {
                    "model": model,
                    "horizon_weeks": h,
                    "mae": float(np.mean(abs_errors)),
                    "mape": float(np.mean(pct_errors)),
                    "n_forecasts": len(abs_errors),
                }
            )
    backtest_frame = pl.DataFrame(
        backtests,
        schema={
            "model": pl.String,
            "horizon_weeks": pl.Int64,
            "mae": pl.Float64,
            "mape": pl.Float64,
            "n_forecasts": pl.Int64,
        },
    )

    model = choose_model(backtest_frame)
    # Sem o Brent das semanas recentes, a regressão não tem como prever: vale
    # o naive, que só precisa do último preço.
    if model != "naive" and _predict(model, x, log_p, base, 1) is None:
        model = "naive"
    rows = []
    for h in HORIZONS:
        predicted = _predict(model, x, log_p, base, h)
        if predicted is None or (model, h) not in errors:
            continue
        low, high = np.quantile(errors[model, h], [LOWER_Q, UPPER_Q])
        value = np.exp(predicted)
        rows.append(
            {
                "week_start": weeks[base] + timedelta(weeks=h),
                "horizon_weeks": h,
                "predicted": round(float(value), 3),
                # Os erros podem ser todos de um lado (modelo enviesado): o
                # intervalo sempre contém a previsão.
                "lower_80": round(float(min(value, np.exp(predicted + low))), 3),
                "upper_80": round(float(max(value, np.exp(predicted + high))), 3),
            }
        )
    forecasts = pl.DataFrame(
        rows,
        schema={
            "week_start": pl.Date,
            "horizon_weeks": pl.Int64,
            "predicted": pl.Float64,
            "lower_80": pl.Float64,
            "upper_80": pl.Float64,
        },
    )
    return Result(model, weeks[base], forecasts, backtest_frame)


def choose_model(backtests: pl.DataFrame) -> str:
    """O modelo de menor erro absoluto médio somando todos os horizontes; em
    empate (ou sem backtest de algum), o naive, que é o mais simples."""
    mean_mae = dict(backtests.group_by("model").agg(pl.col("mae").mean()).iter_rows())
    best = "naive"
    for model in MODELS:
        if model in mean_mae and mean_mae[model] < mean_mae.get(best, float("inf")):
            best = model
    return best
