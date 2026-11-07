import type { EChartsOption } from "echarts";

import { formatPrice } from "@/lib/fuels";
import { formatDate, formatMonth } from "@/lib/indicators";
import type { ChartColors } from "../chart-option";

/** Uma linha do gráfico semanal: o Brasil ou uma UF. */
export interface WeeklySeries {
  id: string;
  name: string; // "São Paulo", "Brasil"
  shortName: string; // rótulo no fim da linha: "SP", "Brasil"
  points: [week: string, price: number | null][]; // null = semana sem dado
  emphasis: boolean;
}

const DAY_MS = 24 * 60 * 60 * 1000;
const axisPrice = new Intl.NumberFormat("pt-BR", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/**
 * Opções do ECharts para o preço semanal. Com uma UF escolhida, a linha dela
 * fica na cor da série e a do Brasil em cinza, como referência: a cor marca o
 * que a pessoa escolheu ver.
 */
export function buildWeeklyOption(series: WeeklySeries[], title: string, colors: ChartColors): EChartsOption {
  const weeks = series.flatMap((s) => s.points.map(([week]) => week)).toSorted();
  const start = weeks[0];
  const end = weeks[weeks.length - 1];
  const main = series.find((s) => s.emphasis) ?? series[0];
  const lastOf = (s: WeeklySeries) => s.points.findLast(([, price]) => price !== null);
  const mainLast = lastOf(main);

  return {
    animation: false,
    textStyle: { fontFamily: colors.font },
    aria: {
      enabled: true,
      label: {
        description:
          mainLast && mainLast[1] !== null
            ? `${title}: preço médio semanal de ${formatDate(start)} a ${formatDate(end)}. ` +
              `${main.name}, semana de ${formatDate(mainLast[0])}: ${formatPrice(mainLast[1])}.`
            : `${title}: sem dados no período.`,
      },
    },
    // Espaço à direita para o rótulo no fim de cada linha ("SP R$ 6,62").
    grid: { left: 4, right: 116, top: 20, bottom: 4, containLabel: true },
    xAxis: {
      type: "time",
      min: start,
      max: end,
      minInterval: 28 * DAY_MS, // no máximo um rótulo por mês
      axisLine: { lineStyle: { color: colors.axis } },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: {
        color: colors.muted,
        hideOverlap: true,
        formatter: (ts: number) => {
          const d = new Date(ts);
          return formatMonth(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`);
        },
      },
    },
    yAxis: {
      type: "value",
      scale: true, // preço não precisa partir do zero numa linha
      splitNumber: 4,
      axisLabel: { color: colors.muted, formatter: (v: number) => axisPrice.format(v) },
      splitLine: { lineStyle: { color: colors.grid, width: 1, type: "solid" } },
    },
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "line", lineStyle: { color: colors.axis, width: 1 } },
      backgroundColor: colors.surface,
      borderColor: colors.hairline,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: colors.ink, fontFamily: colors.font, fontSize: 13 },
      extraCssText: "border-radius: 8px; box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);",
      // Os valores primeiro e em destaque; a semana, por último.
      formatter: (params) => {
        const list = (Array.isArray(params) ? params : [params]) as unknown as {
          seriesIndex: number;
          value: [string, number | null];
        }[];
        const rows = list
          .filter((p) => p.seriesIndex < series.length)
          .map((p) => {
            const price = p.value[1];
            return (
              `<div style="display:flex;align-items:baseline;gap:8px">` +
              `<span style="font-weight:600;font-size:15px">${price === null ? "—" : formatPrice(price)}</span>` +
              `<span style="color:${colors.ink2}">${series[p.seriesIndex].name}</span></div>`
            );
          });
        const week = list[0]?.value[0];
        return rows.join("") + (week ? `<div style="color:${colors.ink2}">semana de ${formatDate(week)}</div>` : "");
      },
    },
    series: [
      ...series.map((s) => {
        const color = s.emphasis ? colors.series : colors.muted;
        const last = lastOf(s);
        return {
          type: "line" as const,
          name: s.name,
          data: s.points,
          // Sem dado, a linha para: ligar os pontos por cima de um buraco
          // inventaria preços para semanas que a ANP não pesquisou.
          connectNulls: false,
          showSymbol: false,
          z: s.emphasis ? 3 : 2,
          lineStyle: { width: s.emphasis ? 2 : 1.5, color, cap: "round" as const, join: "round" as const },
          itemStyle: { color },
          emphasis: { disabled: true },
          endLabel:
            last && last[1] !== null
              ? {
                  show: true,
                  formatter: `${s.shortName} ${formatPrice(last[1])}`,
                  color: s.emphasis ? colors.ink : colors.ink2,
                  fontWeight: s.emphasis ? 600 : 400,
                  distance: 10,
                }
              : undefined,
          // Rótulos de valores próximos (UF e Brasil quase iguais) não se sobrepõem.
          labelLayout: { moveOverlap: "shiftY" as const },
        };
      }),
      // Ponto no último valor da linha em destaque, com anel da cor da superfície.
      {
        type: "line" as const,
        data: mainLast && mainLast[1] !== null ? [mainLast] : [],
        symbol: "circle",
        symbolSize: 8,
        showSymbol: true,
        itemStyle: { color: colors.series, borderColor: colors.surface, borderWidth: 2 },
        lineStyle: { width: 0 },
        silent: true,
        tooltip: { show: false },
        emphasis: { disabled: true },
        z: 4,
      },
    ],
  };
}
