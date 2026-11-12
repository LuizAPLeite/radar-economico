/**
 * Lógica da página de análises: funções puras, sem rede e sem DOM, testadas
 * com `node --test` (ver analysis.test.ts).
 */
import type { Backtest, Driver, LagCorrelation, Regime, RollingCorrelation } from "./api";
import { formatNumber } from "./indicators.ts";
import type { SearchParams } from "./fuels.ts";

// --- Combustíveis analisados ------------------------------------------------------

/**
 * Os combustíveis que o pipeline analisa, na ordem dos botões. `of` é o nome
 * com a preposição certa para o meio da frase ("o preço do diesel S10").
 */
export const ANALYSIS_FUELS = [
  { id: "gasolina", label: "Gasolina", of: "da gasolina" },
  { id: "diesel_s10", label: "Diesel S10", of: "do diesel S10" },
  { id: "diesel", label: "Diesel S500", of: "do diesel S500" },
  { id: "etanol", label: "Etanol", of: "do etanol" },
] as const;

export type AnalysisFuel = (typeof ANALYSIS_FUELS)[number]["id"];

/** ?combustivel=etanol; valor ausente ou fora da lista vira gasolina. */
export function parseAnalysisFuel(params: SearchParams): AnalysisFuel {
  const raw = Array.isArray(params.combustivel) ? params.combustivel[0] : params.combustivel;
  return ANALYSIS_FUELS.find((f) => f.id === raw)?.id ?? "gasolina";
}

export function analysisHref(fuel: AnalysisFuel): string {
  return fuel === "gasolina" ? "/analises" : `/analises?combustivel=${fuel}`;
}

// --- Fatores ----------------------------------------------------------------------

export const DRIVER_LABELS: Record<Driver, string> = {
  brent_brl: "Petróleo em reais",
  brent_usd: "Petróleo em dólares",
  usd_brl: "Dólar",
};

/** Correlação no padrão brasileiro, com o sinal de menos tipográfico: "0,41", "−0,07". */
export function formatCorrelation(value: number): string {
  const text = formatNumber(Math.abs(value), 2);
  return value < 0 && text !== "0,00" ? `−${text}` : text;
}

/** "1 semana", "3 semanas", "na mesma semana". */
export function formatLag(weeks: number): string {
  if (weeks === 0) return "na mesma semana";
  return weeks === 1 ? "1 semana depois" : `${weeks} semanas depois`;
}

// --- Correlações ------------------------------------------------------------------

export interface RegimeBest {
  regime: Regime;
  lag: number;
  correlation: number;
}

/** Para cada período (na ordem dada), a defasagem de maior correlação de um fator. */
export function bestByRegime(lags: LagCorrelation[], driver: Driver, regimes: Regime[]): RegimeBest[] {
  return regimes.flatMap((regime) => {
    const rows = lags.filter((l) => l.driver === driver && l.regime_id === regime.id);
    if (rows.length === 0) return [];
    const best = rows.reduce((a, b) => (b.correlation > a.correlation ? b : a));
    return [{ regime, lag: best.lag_weeks, correlation: best.correlation }];
  });
}

/** Correlação em cada defasagem (0, 1, 2...) de um fator num período. */
export function lagProfile(lags: LagCorrelation[], driver: Driver, regimeId: string): [lag: number, corr: number][] {
  return lags
    .filter((l) => l.driver === driver && l.regime_id === regimeId)
    .toSorted((a, b) => a.lag_weeks - b.lag_weeks)
    .map((l) => [l.lag_weeks, l.correlation]);
}

/** "2016–2023", ou "Desde 2023" no período atual (o fim é exclusivo: 16/05/2023 conta como 2023). */
export function regimeShortName(regime: Regime): string {
  const from = regime.starts_on.slice(0, 4);
  return regime.ends_on === null ? `Desde ${from}` : `${from}–${regime.ends_on.slice(0, 4)}`;
}

/** O período atual (sem data de fim); se não houver, o mais recente. */
export function currentRegime(regimes: Regime[]): Regime | null {
  return regimes.find((r) => r.ends_on === null) ?? regimes.at(-1) ?? null;
}

/**
 * Série da correlação móvel de um fator, semana a semana, com null nas
 * semanas que faltam (janelas sem pares suficientes): a linha se interrompe
 * em vez de ligar pontos por cima do buraco.
 */
export function rollingSeries(rolling: RollingCorrelation[], driver: Driver): [week: string, corr: number | null][] {
  const rows = rolling.filter((r) => r.driver === driver).toSorted((a, b) => a.week_start.localeCompare(b.week_start));
  if (rows.length === 0) return [];
  const byWeek = new Map(rows.map((r) => [r.week_start, r.correlation]));
  const out: [string, number | null][] = [];
  const last = rows[rows.length - 1].week_start;
  for (let week = rows[0].week_start; week <= last; week = addDays(week, 7)) {
    out.push([week, byWeek.get(week) ?? null]);
  }
  return out;
}

function addDays(iso: string, days: number): string {
  const d = new Date(`${iso}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

// --- Previsão e backtest -----------------------------------------------------------

export const MODEL_LABELS: Record<string, string> = {
  naive: "Preço fica igual",
  brent: "Regressão com o petróleo",
};

export interface BacktestRow {
  horizon: number;
  naive: number | null; // erro absoluto médio (R$)
  brent: number | null;
}

export interface BacktestSummary {
  rows: BacktestRow[];
  meanNaive: number | null;
  meanBrent: number | null;
  /** Quanto o modelo do petróleo erra a mais (+) ou a menos (−) que o naive, em %. */
  brentVsNaivePercent: number | null;
}

export function summarizeBacktests(backtests: Backtest[]): BacktestSummary {
  const horizons = [...new Set(backtests.map((b) => b.horizon_weeks))].toSorted((a, b) => a - b);
  const mae = (model: string, h: number) =>
    backtests.find((b) => b.model === model && b.horizon_weeks === h)?.mae ?? null;
  const rows = horizons.map((h) => ({ horizon: h, naive: mae("naive", h), brent: mae("brent", h) }));
  const mean = (values: (number | null)[]) => {
    const present = values.filter((v): v is number => v !== null);
    return present.length === 0 ? null : present.reduce((a, b) => a + b, 0) / present.length;
  };
  const meanNaive = mean(rows.map((r) => r.naive));
  const meanBrent = mean(rows.map((r) => r.brent));
  return {
    rows,
    meanNaive,
    meanBrent,
    brentVsNaivePercent:
      meanNaive !== null && meanBrent !== null && meanNaive > 0 ? (meanBrent / meanNaive - 1) * 100 : null,
  };
}
