import type { EChartsOption } from "echarts";

import { formatCorrelation } from "@/lib/analysis";
import { formatPrice } from "@/lib/fuels";
import { formatDate } from "@/lib/indicators";
import type { ChartColors } from "../chart-option";

/*
 * Opções dos gráficos da página de análises. Em todos, a cor de destaque é o
 * petróleo em reais (o fator que explica a bomba) e o cinza é a referência
 * (o dólar sozinho, ou o passado na previsão). Duas cores, sempre com
 * legenda e rótulo direto: a identidade nunca depende só da cor.
 */

const DAY_MS = 24 * 60 * 60 * 1000;

function tooltipBase(colors: ChartColors) {
  return {
    backgroundColor: colors.surface,
    borderColor: colors.hairline,
    borderWidth: 1,
    padding: [8, 12] as [number, number],
    textStyle: { color: colors.ink, fontFamily: colors.font, fontSize: 13 },
    extraCssText: "border-radius: 8px; box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);",
  };
}

function tooltipRow(value: string, label: string, colors: ChartColors): string {
  return (
    `<div style="display:flex;align-items:baseline;gap:8px">` +
    `<span style="font-weight:600;font-size:15px">${value}</span>` +
    `<span style="color:${colors.ink2}">${label}</span></div>`
  );
}

/**
 * Eixo de correlação com marcas a cada `step` e limites alinhados a elas (um
 * limite fora das marcas deixa rótulos colados). Sempre inclui o zero e vai
 * pelo menos de `floor` a `ceiling`, para gráficos de combustíveis diferentes
 * terem escalas comparáveis.
 */
const correlationAxis = (colors: ChartColors, floor: number, ceiling: number, step = 0.2) => ({
  type: "value" as const,
  interval: step,
  min: (v: { min: number }) => Math.min(floor, Math.floor(v.min / step) * step),
  max: (v: { max: number }) => Math.max(ceiling, Math.ceil(v.max / step) * step),
  axisLabel: { color: colors.muted, formatter: (v: number) => formatCorrelation(v) },
  splitLine: { lineStyle: { color: colors.grid, width: 1, type: "solid" as const } },
});

// --- 1. Correlação por período ----------------------------------------------------

export interface RegimeBarsData {
  regimes: string[]; // nomes curtos, na ordem do tempo
  main: { name: string; values: (number | null)[]; lags: (number | null)[] };
  reference: { name: string; values: (number | null)[]; lags: (number | null)[] };
}

/** Barras agrupadas: a maior correlação de cada fator em cada período. */
export function buildRegimeBarsOption(data: RegimeBarsData, title: string, colors: ChartColors): EChartsOption {
  const bar = (s: RegimeBarsData["main"], color: string, emphasis: boolean) => ({
    type: "bar" as const,
    name: s.name,
    barMaxWidth: 40,
    barGap: "8%", // fresta entre as barras do mesmo período
    emphasis: { disabled: true },
    data: s.values.map((value) => ({
      value,
      itemStyle: {
        color,
        // Ponta arredondada do lado do valor; reta na linha do zero.
        borderRadius: (value ?? 0) < 0 ? [0, 0, 4, 4] : [4, 4, 0, 0],
      },
    })),
    label: {
      show: true,
      position: "top" as const,
      color: emphasis ? colors.ink : colors.ink2,
      fontWeight: emphasis ? 600 : 400,
      formatter: (p: { dataIndex: number }) => {
        const value = s.values[p.dataIndex];
        return value === null ? "" : formatCorrelation(value);
      },
    },
  });
  return {
    animation: false,
    textStyle: { fontFamily: colors.font },
    aria: {
      enabled: true,
      label: {
        description:
          `${title}. ` +
          data.regimes
            .map((r, i) => {
              const m = data.main.values[i];
              const ref = data.reference.values[i];
              return `${r}: ${data.main.name} ${m === null ? "sem dados" : formatCorrelation(m)}, ${data.reference.name} ${ref === null ? "sem dados" : formatCorrelation(ref)}`;
            })
            .join("; ") +
          ".",
      },
    },
    grid: { left: 4, right: 4, top: 28, bottom: 4, containLabel: true },
    xAxis: {
      type: "category",
      data: data.regimes,
      axisLine: { lineStyle: { color: colors.axis } },
      axisTick: { show: false },
      axisLabel: { color: colors.ink2, interval: 0 },
    },
    yAxis: correlationAxis(colors, 0, 0.6),
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "shadow", shadowStyle: { color: colors.hover } },
      ...tooltipBase(colors),
      formatter: (params) => {
        const list = (Array.isArray(params) ? params : [params]) as unknown as { dataIndex: number; seriesIndex: number }[];
        const i = list[0]?.dataIndex ?? 0;
        const rows = [data.main, data.reference].map((s) => {
          const v = s.values[i];
          const lag = s.lags[i];
          const lagText = lag === null ? "" : ` (${lag === 0 ? "mesma semana" : `${lag} sem. antes`})`;
          return tooltipRow(v === null ? "—" : formatCorrelation(v), `${s.name}${lagText}`, colors);
        });
        return rows.join("") + `<div style="color:${colors.ink2}">${data.regimes[i]}</div>`;
      },
    },
    series: [bar(data.main, colors.series, true), bar(data.reference, colors.muted, false)],
  };
}

// --- 2. Correlação por defasagem ----------------------------------------------------

export interface LagData {
  main: { name: string; points: [lag: number, corr: number][] };
  reference: { name: string; points: [lag: number, corr: number][] };
}

/** Linhas com marcadores: a correlação em cada defasagem, de 0 a 12 semanas. */
export function buildLagOption(data: LagData, title: string, colors: ChartColors): EChartsOption {
  const best = data.main.points.reduce<[number, number] | null>((a, p) => (a === null || p[1] > a[1] ? p : a), null);
  const lags = [...new Set([...data.main.points, ...data.reference.points].map(([l]) => l))].toSorted((a, b) => a - b);
  const line = (s: LagData["main"], color: string, emphasis: boolean) => ({
    type: "line" as const,
    name: s.name,
    data: s.points,
    symbol: "circle",
    symbolSize: emphasis ? 8 : 6,
    showSymbol: true,
    itemStyle: { color, borderColor: colors.surface, borderWidth: 2 },
    lineStyle: { width: 2, color },
    emphasis: { disabled: true },
    z: emphasis ? 3 : 2,
  });
  return {
    animation: false,
    textStyle: { fontFamily: colors.font },
    aria: {
      enabled: true,
      label: {
        description: best
          ? `${title}. Maior correlação do ${data.main.name.toLowerCase()}: ${formatCorrelation(best[1])} com ${best[0]} semanas de defasagem.`
          : `${title}: sem dados.`,
      },
    },
    grid: { left: 4, right: 12, top: 28, bottom: 4, containLabel: true },
    xAxis: {
      type: "value",
      min: lags[0] ?? 0,
      max: lags.at(-1) ?? 12,
      interval: 1,
      name: "semanas de defasagem",
      nameLocation: "middle",
      nameGap: 28,
      nameTextStyle: { color: colors.ink2 },
      axisLine: { lineStyle: { color: colors.axis } },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: { color: colors.muted },
    },
    yAxis: correlationAxis(colors, 0, 0.6),
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "line", lineStyle: { color: colors.axis, width: 1 } },
      ...tooltipBase(colors),
      formatter: (params) => {
        const list = (Array.isArray(params) ? params : [params]) as unknown as {
          seriesIndex: number;
          value: [number, number];
        }[];
        const names = [data.main.name, data.reference.name];
        const rows = list
          .filter((p) => p.seriesIndex < 2)
          .map((p) => tooltipRow(formatCorrelation(p.value[1]), names[p.seriesIndex], colors));
        const lag = list[0]?.value[0];
        return rows.join("") + `<div style="color:${colors.ink2}">${lag} semana${lag === 1 ? "" : "s"} de defasagem</div>`;
      },
    },
    series: [
      line(data.main, colors.series, true),
      line(data.reference, colors.muted, false),
      // Rótulo só no ponto de maior correlação do fator principal.
      {
        type: "line" as const,
        data: best ? [best] : [],
        symbol: "circle",
        symbolSize: 10,
        itemStyle: { color: colors.series, borderColor: colors.surface, borderWidth: 2 },
        lineStyle: { width: 0 },
        label: {
          show: true,
          position: "top",
          color: colors.ink,
          fontWeight: 600,
          formatter: () => (best ? formatCorrelation(best[1]) : ""),
        },
        silent: true,
        tooltip: { show: false },
        emphasis: { disabled: true },
        z: 4,
      },
    ],
  };
}

// --- 3. Correlação móvel ------------------------------------------------------------

export interface RollingData {
  main: { name: string; points: [week: string, corr: number | null][] };
  reference: { name: string; points: [week: string, corr: number | null][] };
  /** Começo de cada período depois do primeiro: uma linha vertical com o nome. */
  boundaries: { date: string; label: string }[];
}

export function buildRollingOption(data: RollingData, title: string, colors: ChartColors): EChartsOption {
  const weeks = [...data.main.points, ...data.reference.points].map(([w]) => w).toSorted();
  const start = weeks[0];
  const end = weeks.at(-1);
  const lastMain = data.main.points.findLast(([, v]) => v !== null);
  const line = (s: RollingData["main"], color: string, emphasis: boolean, withMarks: boolean) => ({
    type: "line" as const,
    name: s.name,
    data: s.points,
    connectNulls: false,
    showSymbol: false,
    lineStyle: { width: emphasis ? 2 : 1.5, color, cap: "round" as const, join: "round" as const },
    itemStyle: { color },
    emphasis: { disabled: true },
    z: emphasis ? 3 : 2,
    markLine: withMarks
      ? {
          silent: true,
          symbol: "none",
          lineStyle: { color: colors.axis, type: "dashed" as const, width: 1 },
          label: { color: colors.ink2, position: "insideEndTop" as const, formatter: (p: { name?: string }) => p.name ?? "" },
          data: [
            ...data.boundaries.map((b) => ({ name: b.label, xAxis: b.date })),
            // Linha do zero: acima, os dois sobem juntos; abaixo, em direções opostas.
            { name: "", yAxis: 0, lineStyle: { color: colors.axis, type: "solid" as const } },
          ],
        }
      : undefined,
  });
  return {
    animation: false,
    textStyle: { fontFamily: colors.font },
    aria: {
      enabled: true,
      label: {
        description:
          lastMain && lastMain[1] !== null
            ? `${title}, de ${formatDate(start)} a ${formatDate(end ?? start)}. ` +
              `${data.main.name} na janela que termina em ${formatDate(lastMain[0])}: ${formatCorrelation(lastMain[1])}.`
            : `${title}: sem dados.`,
      },
    },
    grid: { left: 4, right: 12, top: 28, bottom: 4, containLabel: true },
    xAxis: {
      type: "time",
      min: start,
      max: end,
      minInterval: 365 * DAY_MS,
      axisLine: { lineStyle: { color: colors.axis } },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: { color: colors.muted, hideOverlap: true, formatter: (ts: number) => String(new Date(ts).getFullYear()) },
    },
    yAxis: correlationAxis(colors, -0.4, 0.8, 0.4),
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "line", lineStyle: { color: colors.axis, width: 1 } },
      ...tooltipBase(colors),
      formatter: (params) => {
        const list = (Array.isArray(params) ? params : [params]) as unknown as {
          seriesIndex: number;
          value: [string, number | null];
        }[];
        const names = [data.main.name, data.reference.name];
        const rows = list
          .filter((p) => p.seriesIndex < 2)
          .map((p) => tooltipRow(p.value[1] === null ? "—" : formatCorrelation(p.value[1]), names[p.seriesIndex], colors));
        const week = list[0]?.value[0];
        return rows.join("") + (week ? `<div style="color:${colors.ink2}">52 semanas até ${formatDate(week)}</div>` : "");
      },
    },
    series: [line(data.main, colors.series, true, true), line(data.reference, colors.muted, false, false)],
  };
}

// --- 4. Previsão ----------------------------------------------------------------------

export interface ForecastData {
  history: [week: string, price: number | null][];
  forecast: { week: string; predicted: number; lower: number; upper: number }[];
}

/**
 * O passado recente em linha cheia; as semanas previstas em linha tracejada a
 * partir do último preço observado, com a faixa de 80% em volta.
 */
export function buildForecastOption(data: ForecastData, title: string, colors: ChartColors): EChartsOption {
  const lastObserved = data.history.findLast(([, p]) => p !== null);
  const anchor = lastObserved && lastObserved[1] !== null ? { week: lastObserved[0], value: lastObserved[1] } : null;
  const ahead = anchor
    ? [{ week: anchor.week, predicted: anchor.value, lower: anchor.value, upper: anchor.value }, ...data.forecast]
    : data.forecast;
  const last = data.forecast.at(-1);
  const start = data.history[0]?.[0] ?? data.forecast[0]?.week;
  const end = last?.week ?? anchor?.week;

  return {
    animation: false,
    textStyle: { fontFamily: colors.font },
    aria: {
      enabled: true,
      label: {
        description: last
          ? `${title}. Último preço observado: ${anchor ? `${formatPrice(anchor.value)} na semana de ${formatDate(anchor.week)}` : "sem dados"}. ` +
            `Previsão para a semana de ${formatDate(last.week)}: ${formatPrice(last.predicted)}, ` +
            `com 80% de chance entre ${formatPrice(last.lower)} e ${formatPrice(last.upper)}.`
          : `${title}: sem previsão calculada.`,
      },
    },
    grid: { left: 4, right: 88, top: 20, bottom: 4, containLabel: true },
    xAxis: {
      type: "time",
      min: start,
      max: end,
      minInterval: 28 * DAY_MS,
      axisLine: { lineStyle: { color: colors.axis } },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: {
        color: colors.muted,
        hideOverlap: true,
        formatter: (ts: number) => {
          const d = new Date(ts);
          return `${["jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"][d.getMonth()]}/${String(d.getFullYear()).slice(2)}`;
        },
      },
    },
    yAxis: {
      type: "value",
      scale: true,
      splitNumber: 4,
      axisLabel: { color: colors.muted, formatter: (v: number) => formatPrice(v).replace("R$ ", "") },
      splitLine: { lineStyle: { color: colors.grid, width: 1, type: "solid" } },
    },
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "line", lineStyle: { color: colors.axis, width: 1 } },
      ...tooltipBase(colors),
      formatter: (params) => {
        const list = (Array.isArray(params) ? params : [params]) as unknown as {
          seriesIndex: number;
          value: [string, number | null];
        }[];
        const week = list[0]?.value[0];
        const point = data.forecast.find((f) => f.week === week);
        if (point) {
          return (
            tooltipRow(formatPrice(point.predicted), "previsão", colors) +
            `<div style="color:${colors.ink2}">80%: ${formatPrice(point.lower)} a ${formatPrice(point.upper)}</div>` +
            `<div style="color:${colors.ink2}">semana de ${formatDate(point.week)}</div>`
          );
        }
        const observed = list.find((p) => p.seriesIndex === 0)?.value[1];
        if (observed === undefined || observed === null || !week) return "";
        return tooltipRow(formatPrice(observed), "preço médio", colors) + `<div style="color:${colors.ink2}">semana de ${formatDate(week)}</div>`;
      },
    },
    series: [
      {
        type: "line",
        name: "Preço médio",
        data: data.history,
        connectNulls: false,
        showSymbol: false,
        lineStyle: { width: 2, color: colors.series, cap: "round", join: "round" },
        itemStyle: { color: colors.series },
        emphasis: { disabled: true },
        z: 3,
      },
      // Faixa de 80%: uma área invisível até o limite de baixo e, empilhada
      // sobre ela, a área da largura da faixa.
      {
        type: "line",
        data: ahead.map((f) => [f.week, f.lower]),
        stack: "faixa",
        showSymbol: false,
        lineStyle: { width: 0 },
        silent: true,
        tooltip: { show: false },
        emphasis: { disabled: true },
      },
      {
        type: "line",
        name: "Faixa de 80%",
        data: ahead.map((f) => [f.week, f.upper - f.lower]),
        stack: "faixa",
        showSymbol: false,
        lineStyle: { width: 0 },
        areaStyle: { color: colors.series, opacity: 0.16 },
        silent: true,
        tooltip: { show: false },
        emphasis: { disabled: true },
      },
      {
        type: "line",
        name: "Previsão",
        data: ahead.map((f) => [f.week, f.predicted]),
        showSymbol: false,
        lineStyle: { width: 2, color: colors.series, type: [4, 4] },
        itemStyle: { color: colors.series },
        emphasis: { disabled: true },
        endLabel: last
          ? { show: true, formatter: () => formatPrice(last.predicted), color: colors.ink, fontWeight: 600, distance: 10 }
          : undefined,
        z: 3,
      },
    ],
  };
}
