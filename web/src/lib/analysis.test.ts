import assert from "node:assert/strict";
import { describe, test } from "node:test";

import type { Backtest, LagCorrelation, Regime, RollingCorrelation } from "./api.ts";
import {
  ANALYSIS_FUELS,
  analysisHref,
  bestByRegime,
  currentRegime,
  formatCorrelation,
  formatLag,
  lagProfile,
  parseAnalysisFuel,
  regimeShortName,
  rollingSeries,
  summarizeBacktests,
} from "./analysis.ts";

const regime = (id: string, starts: string, ends: string | null): Regime => ({
  id,
  name: id,
  starts_on: starts,
  ends_on: ends,
  description: "",
});
const REGIMES = [regime("ppi", "2016-10-14", "2023-05-16"), regime("atual", "2023-05-16", null)];

const lag = (driver: LagCorrelation["driver"], regimeId: string, weeks: number, corr: number): LagCorrelation => ({
  driver,
  regime_id: regimeId,
  lag_weeks: weeks,
  correlation: corr,
  n_weeks: 100,
});
const LAGS = [
  lag("brent_brl", "atual", 0, 0.2),
  lag("brent_brl", "atual", 2, 0.41),
  lag("brent_brl", "atual", 1, 0.35),
  lag("brent_brl", "ppi", 3, 0.49),
  lag("usd_brl", "atual", 0, 0.14),
];

describe("filtro", () => {
  test("combustível da URL, com gasolina como padrão", () => {
    assert.equal(parseAnalysisFuel({ combustivel: "etanol" }), "etanol");
    assert.equal(parseAnalysisFuel({ combustivel: ["diesel_s10", "etanol"] }), "diesel_s10");
    assert.equal(parseAnalysisFuel({ combustivel: "gnv" }), "gasolina"); // não analisado
    assert.equal(parseAnalysisFuel({}), "gasolina");
  });

  test("o link da gasolina não leva parâmetro", () => {
    assert.equal(analysisHref("gasolina"), "/analises");
    assert.equal(analysisHref("etanol"), "/analises?combustivel=etanol");
  });
});

describe("formatação", () => {
  test("correlação com vírgula e sinal de menos tipográfico", () => {
    assert.equal(formatCorrelation(0.4123), "0,41");
    assert.equal(formatCorrelation(-0.07), "−0,07");
    assert.equal(formatCorrelation(-0.001), "0,00");
  });

  test("defasagem por extenso", () => {
    assert.equal(formatLag(0), "na mesma semana");
    assert.equal(formatLag(1), "1 semana depois");
    assert.equal(formatLag(3), "3 semanas depois");
  });
});

describe("correlações", () => {
  test("melhor defasagem de cada período, na ordem dos períodos", () => {
    assert.deepEqual(
      bestByRegime(LAGS, "brent_brl", REGIMES).map((b) => [b.regime.id, b.lag, b.correlation]),
      [
        ["ppi", 3, 0.49],
        ["atual", 2, 0.41],
      ],
    );
    // Período sem dados do fator fica de fora.
    assert.deepEqual(
      bestByRegime(LAGS, "usd_brl", REGIMES).map((b) => b.regime.id),
      ["atual"],
    );
  });

  test("perfil por defasagem em ordem", () => {
    assert.deepEqual(lagProfile(LAGS, "brent_brl", "atual"), [
      [0, 0.2],
      [1, 0.35],
      [2, 0.41],
    ]);
  });

  test("nome curto do período", () => {
    assert.equal(regimeShortName(REGIMES[0]), "2016–2023");
    assert.equal(regimeShortName(REGIMES[1]), "Desde 2023");
  });

  test("período atual é o sem fim", () => {
    assert.equal(currentRegime(REGIMES)?.id, "atual");
    assert.equal(currentRegime([]), null);
  });

  test("correlação móvel com null nas semanas que faltam", () => {
    const rolling: RollingCorrelation[] = [
      { driver: "brent_brl", week_start: "2026-10-19", lag_weeks: 2, correlation: 0.3 },
      { driver: "brent_brl", week_start: "2026-10-05", lag_weeks: 2, correlation: 0.2 },
      { driver: "usd_brl", week_start: "2026-10-12", lag_weeks: 1, correlation: 0.1 },
    ];
    assert.deepEqual(rollingSeries(rolling, "brent_brl"), [
      ["2026-10-05", 0.2],
      ["2026-10-12", null],
      ["2026-10-19", 0.3],
    ]);
    assert.deepEqual(rollingSeries([], "brent_brl"), []);
  });
});

describe("backtest", () => {
  const bt = (model: string, h: number, mae: number): Backtest => ({
    model,
    horizon_weeks: h,
    mae,
    mape: 0,
    n_forecasts: 104,
  });

  test("tabela por horizonte e quanto o petróleo erra a mais que o naive", () => {
    const summary = summarizeBacktests([bt("naive", 2, 0.04), bt("brent", 1, 0.03), bt("naive", 1, 0.02), bt("brent", 2, 0.05)]);
    assert.deepEqual(summary.rows, [
      { horizon: 1, naive: 0.02, brent: 0.03 },
      { horizon: 2, naive: 0.04, brent: 0.05 },
    ]);
    assert.ok(Math.abs((summary.meanNaive ?? 0) - 0.03) < 1e-12);
    assert.ok(Math.abs((summary.brentVsNaivePercent ?? 0) - 33.333) < 0.01);
  });

  test("sem backtest, tudo vazio", () => {
    assert.deepEqual(summarizeBacktests([]), { rows: [], meanNaive: null, meanBrent: null, brentVsNaivePercent: null });
  });
});

describe("catálogo", () => {
  test("cada combustível tem o nome com a preposição para a frase", () => {
    assert.deepEqual(
      ANALYSIS_FUELS.map((f) => f.of),
      ["da gasolina", "do diesel S10", "do diesel S500", "do etanol"],
    );
  });
});
