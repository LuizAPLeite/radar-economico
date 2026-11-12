/**
 * Cálculos e formatação dos indicadores: funções puras, sem rede e sem DOM.
 * Ficam fora dos componentes para serem testadas com `node --test`
 * (ver indicators.test.ts).
 */
import type { IndicatorWithLatest, Point } from "./api";

// --- Formatação (pt-BR) -------------------------------------------------------

const MONTHS = ["jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"];

/**
 * "2026-11-06" → "06/11/2026".
 *
 * Trabalha direto no texto de propósito: `new Date("2026-11-06")` é meia-noite
 * em UTC, que no horário de Brasília ainda é dia 05 — a data sairia errada.
 */
export function formatDate(iso: string): string {
  const [year, month, day] = iso.split("-");
  return `${day}/${month}/${year}`;
}

/** "2026-09-01" → "set/2026". */
export function formatMonth(iso: string): string {
  const [year, month] = iso.split("-");
  return `${MONTHS[Number(month) - 1]}/${year}`;
}

/** 5.5013 → "5,5013" (com exatamente `decimals` casas). */
export function formatNumber(value: number, decimals: number): string {
  return new Intl.NumberFormat("pt-BR", {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(value);
}

/** Número com sinal: "+0,25", "-0,32"; zero fica sem sinal. */
export function formatSigned(value: number, decimals: number): string {
  return new Intl.NumberFormat("pt-BR", {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
    signDisplay: "exceptZero",
  }).format(value);
}

/** "2026-11-07T18:39:18Z" → "07/11/2026 às 15:39" (horário de Brasília). */
export function formatUpdatedAt(timestamp: string): string {
  const parts = new Intl.DateTimeFormat("pt-BR", {
    timeZone: "America/Sao_Paulo",
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(timestamp));
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((p) => p.type === type)?.value;
  return `${part("day")}/${part("month")}/${part("year")} às ${part("hour")}:${part("minute")}`;
}

/** Valor de um indicador com a sua unidade: "13,75%", "R$ 5,5013". */
export function formatIndicatorValue(id: string, value: number): string {
  switch (id) {
    case "usd_brl":
      return `R$ ${formatNumber(value, 4)}`;
    case "brent":
      return `US$ ${formatNumber(value, 2)}`;
    case "selic":
    case "ipca":
      return `${formatNumber(value, 2)}%`;
    default:
      return formatNumber(value, 2);
  }
}

// --- Cálculos -----------------------------------------------------------------

/** Variação percentual do último valor em relação ao anterior. */
export function lastChangePercent(points: Point[]): { percent: number; previousDate: string } | null {
  if (points.length < 2) return null;
  const previous = points[points.length - 2];
  const last = points[points.length - 1];
  if (previous.value === 0) return null;
  return {
    percent: ((last.value - previous.value) / previous.value) * 100,
    previousDate: previous.date,
  };
}

/** A mudança de patamar mais recente (na Selic, a última decisão do Copom). */
export function lastLevelChange(points: Point[]): { date: string; delta: number } | null {
  for (let i = points.length - 1; i > 0; i--) {
    const delta = points[i].value - points[i - 1].value;
    if (delta !== 0) {
      // Arredonda para não exibir resíduo de ponto flutuante (0,25000000001).
      return { date: points[i].date, delta: Math.round(delta * 10_000) / 10_000 };
    }
  }
  return null;
}

/**
 * Acumulado em 12 meses de uma série de variações mensais, como o IPCA:
 * (1 + v1/100) × (1 + v2/100) × … × (1 + v12/100) − 1.
 * Variações se compõem (multiplicam), não se somam. Exige 12 meses seguidos.
 */
export function accumulated12Months(points: Point[]): number | null {
  if (points.length < 12) return null;
  const last12 = points.slice(-12);
  for (let i = 1; i < last12.length; i++) {
    if (monthIndex(last12[i].date) - monthIndex(last12[i - 1].date) !== 1) {
      return null; // falta algum mês no meio
    }
  }
  const factor = last12.reduce((acc, p) => acc * (1 + p.value / 100), 1);
  return (factor - 1) * 100;
}

function monthIndex(iso: string): number {
  const [year, month] = iso.split("-").map(Number);
  return year * 12 + (month - 1);
}

// --- Cards --------------------------------------------------------------------

export interface Card {
  id: string;
  label: string;
  value: string;
  /** Contexto do valor: "ao ano em 07/11/2026", "em set/2026". */
  context: string;
  /** Linha de apoio: variação ou acumulado. */
  detail: string | null;
  /** Sentido da variação, para a seta. null quando o detalhe não é uma variação. */
  direction: "up" | "down" | null;
}

function directionOf(value: number): "up" | "down" | null {
  if (value > 0) return "up";
  if (value < 0) return "down";
  return null;
}

/** Monta o card de um indicador a partir do último valor e da série. */
export function buildCard(indicator: IndicatorWithLatest, series: Point[]): Card {
  const { id, name, latest } = indicator;
  const base = { id, label: name, value: formatIndicatorValue(id, latest.value) };

  switch (id) {
    case "selic": {
      const change = lastLevelChange(series);
      return {
        ...base,
        context: `ao ano em ${formatDate(latest.date)}`,
        detail: change && `${formatSigned(change.delta, 2)} p.p. desde ${formatDate(change.date)}`,
        direction: change && directionOf(change.delta),
      };
    }
    case "ipca": {
      const acc = accumulated12Months(series);
      return {
        ...base,
        context: `em ${formatMonth(latest.date)}`,
        detail: acc === null ? null : `${formatNumber(acc, 2)}% em 12 meses`,
        direction: null,
      };
    }
    // Cotações diárias: a variação em relação ao dia anterior com cotação.
    case "usd_brl":
    case "brent": {
      const change = lastChangePercent(series);
      return {
        ...base,
        context: `em ${formatDate(latest.date)}`,
        detail: change && `${formatSigned(change.percent, 2)}% sobre ${formatDate(change.previousDate)}`,
        direction: change && directionOf(change.percent),
      };
    }
    default:
      return { ...base, context: `${indicator.unit} em ${formatDate(latest.date)}`, detail: null, direction: null };
  }
}

// --- Gráficos -----------------------------------------------------------------

/**
 * Forma do gráfico de cada indicador:
 * - Selic em degraus: ela só muda nas reuniões do Copom; uma linha comum
 *   sugeriria uma transição gradual que não existe;
 * - IPCA em barras: cada mês é uma variação própria, acima ou abaixo de zero;
 * - o resto em linha.
 */
export type ChartKind = "line" | "step" | "bar";

export function chartKind(id: string): ChartKind {
  if (id === "selic") return "step";
  if (id === "ipca") return "bar";
  return "line";
}

// --- Período ------------------------------------------------------------------

export const PERIODS = [
  { id: "1y", label: "1 ano", years: 1 },
  { id: "5y", label: "5 anos", years: 5 },
  { id: "10y", label: "10 anos", years: 10 },
  { id: "all", label: "Desde 2000", years: null },
] as const;

export type PeriodId = (typeof PERIODS)[number]["id"];

/**
 * Início de uma janela de `years` anos que termina em `end` (AAAA-MM-DD).
 * `years` null significa a série toda, a partir de `seriesStart`.
 */
export function periodStart(end: string, years: number | null, seriesStart: string): string {
  if (years === null) return seriesStart;
  const [y, m, d] = end.split("-").map(Number);
  const year = y - years;
  // 29/02 menos um ano vira 28/02: o dia 0 do mês seguinte é o último dia do mês.
  const lastDayOfMonth = new Date(Date.UTC(year, m, 0)).getUTCDate();
  const day = Math.min(d, lastDayOfMonth);
  return `${year}-${String(m).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}
