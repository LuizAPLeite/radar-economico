import type { EChartsOption } from "echarts";

import {
  formatDate,
  formatIndicatorValue,
  formatMonth,
  type ChartKind,
} from "@/lib/indicators";

/**
 * Altura do gráfico, incluindo os rótulos dos eixos. Fica neste módulo (sem
 * "use client") porque o esqueleto, que roda no servidor, também precisa
 * dela: o que um arquivo "use client" exporta chega ao servidor como uma
 * referência para o navegador, e não como o valor.
 */
export const CHART_HEIGHT = 260;

/** Cores do gráfico, lidas das variáveis CSS do tema (ver globals.css). */
export interface ChartColors {
  series: string;
  negative: string;
  surface: string;
  ink: string;
  ink2: string;
  muted: string;
  grid: string;
  axis: string;
  hairline: string;
  hover: string;
  font: string;
}

/** O canvas do ECharts não entende var(--x): lemos o valor atual de cada token. */
export function readChartColors(): ChartColors {
  const style = getComputedStyle(document.documentElement);
  const token = (name: string) => style.getPropertyValue(name).trim();
  return {
    series: token("--series-1"),
    negative: token("--series-negative"),
    surface: token("--surface"),
    ink: token("--ink"),
    ink2: token("--ink-2"),
    muted: token("--muted"),
    grid: token("--grid"),
    axis: token("--axis"),
    hairline: token("--hairline"),
    hover: token("--hover-wash"),
    font: getComputedStyle(document.body).fontFamily,
  };
}

export type Tuple = [date: string, value: number];

interface ChartInput {
  id: string;
  title: string;
  kind: ChartKind;
  points: Tuple[];
  start: string; // AAAA-MM-DD
  end: string;
  colors: ChartColors;
}

const DAY_MS = 24 * 60 * 60 * 1000;
const axisNumber = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 2 });

/** Monta as opções do ECharts para uma série (uma única série por gráfico). */
export function buildChartOption({ id, title, kind, points, start, end, colors }: ChartInput): EChartsOption {
  const last = points.at(-1);
  const years = (Date.parse(end) - Date.parse(start)) / (365.25 * DAY_MS);
  const multiYear = years > 2;
  const formatWhen = (date: string) => (kind === "bar" ? formatMonth(date) : formatDate(date));

  return {
    animation: false, // trocar o período redesenha na hora, sem "piscar"
    textStyle: { fontFamily: colors.font },
    aria: {
      enabled: true,
      label: {
        description: last
          ? `${title}: ${points.length} valores de ${formatDate(start)} a ${formatDate(end)}. ` +
            `Último valor: ${formatIndicatorValue(id, last[1])} em ${formatWhen(last[0])}.`
          : `${title}: sem dados no período.`,
      },
    },
    // containLabel: a altura do gráfico já inclui os rótulos dos eixos.
    grid: { left: 4, right: kind === "bar" ? 12 : 88, top: 20, bottom: 4, containLabel: true },
    xAxis: {
      type: "time",
      min: start,
      max: end,
      // Em janelas de vários anos, um rótulo por ano; senão, por mês.
      minInterval: multiYear ? 365 * DAY_MS : 28 * DAY_MS,
      axisLine: { lineStyle: { color: colors.axis } },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: {
        color: colors.muted,
        hideOverlap: true,
        formatter: (ts: number) => {
          const d = new Date(ts);
          const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-01`;
          return multiYear ? String(d.getFullYear()) : formatMonth(iso);
        },
      },
    },
    yAxis: {
      type: "value",
      // Linhas não precisam partir do zero; barras sim (o comprimento é o valor).
      scale: kind !== "bar",
      splitNumber: 4,
      axisLabel: { color: colors.muted, formatter: (v: number) => axisNumber.format(v) },
      splitLine: { lineStyle: { color: colors.grid, width: 1, type: "solid" } },
    },
    tooltip: {
      trigger: "axis",
      // Linhas: uma linha vertical que acompanha o mouse. Barras: a faixa do mês
      // inteira acende, um alvo bem maior que a barra fina.
      axisPointer:
        kind === "bar"
          ? { type: "shadow", shadowStyle: { color: colors.hover } }
          : { type: "line", lineStyle: { color: colors.axis, width: 1 } },
      backgroundColor: colors.surface,
      borderColor: colors.hairline,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: colors.ink, fontFamily: colors.font, fontSize: 13 },
      extraCssText: "border-radius: 8px; box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);",
      // O valor vem primeiro e em destaque; a data, depois.
      formatter: (params) => {
        const first = (Array.isArray(params) ? params[0] : params) as unknown as { value: Tuple };
        const [date, value] = first.value;
        return (
          `<div style="font-weight:600;font-size:15px">${formatIndicatorValue(id, value)}</div>` +
          `<div style="color:${colors.ink2}">${formatWhen(date)}</div>`
        );
      },
    },
    series: kind === "bar" ? barSeries(id, points, colors) : lineSeries(id, kind, points, colors),
  };
}

function lineSeries(id: string, kind: ChartKind, points: Tuple[], colors: ChartColors): EChartsOption["series"] {
  const last = points.at(-1);
  return [
    {
      type: "line",
      data: points,
      step: kind === "step" ? "end" : false,
      showSymbol: false,
      lineStyle: { width: 2, color: colors.series, cap: "round", join: "round" },
      itemStyle: { color: colors.series },
      emphasis: { disabled: true },
      // Muitos pontos diários: o LTTB reduz o que é desenhado sem perder a forma.
      // Nos degraus não, porque deslocaria a data das mudanças.
      sampling: kind === "line" ? "lttb" : undefined,
      // Rótulo só no último valor, e na cor do texto (nunca na cor da série).
      endLabel: last && {
        show: true,
        formatter: () => formatIndicatorValue(id, last[1]),
        color: colors.ink,
        fontWeight: 600,
        distance: 10,
      },
    },
    // Ponto no último valor, com anel da cor da superfície para destacar da linha.
    {
      type: "line",
      data: last ? [last] : [],
      symbol: "circle",
      symbolSize: 8,
      showSymbol: true,
      itemStyle: { color: colors.series, borderColor: colors.surface, borderWidth: 2 },
      lineStyle: { width: 0 },
      silent: true,
      tooltip: { show: false },
      emphasis: { disabled: true },
    },
  ];
}

function barSeries(id: string, points: Tuple[], colors: ChartColors): EChartsOption["series"] {
  const lastIndex = points.length - 1;
  return [
    {
      type: "bar",
      barMaxWidth: 24,
      barWidth: "60%",
      emphasis: { disabled: true },
      data: points.map(([date, value], i) => {
        const negative = value < 0;
        return {
          value: [date, value],
          itemStyle: {
            color: negative ? colors.negative : colors.series,
            // Ponta arredondada no lado do valor; reta na linha do zero.
            borderRadius: negative ? [0, 0, 4, 4] : [4, 4, 0, 0],
          },
          // Rótulo só na barra mais recente.
          label:
            i === lastIndex
              ? {
                  show: true,
                  position: negative ? "bottom" : "top",
                  formatter: formatIndicatorValue(id, value),
                  color: colors.ink,
                  fontWeight: 600,
                }
              : undefined,
        };
      }),
    },
  ];
}
