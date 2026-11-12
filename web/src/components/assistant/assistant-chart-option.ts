import type { EChartsOption } from "echarts";

import { formatDateAs, type ChartData } from "@/lib/assistant";
import type { ChartColors } from "../chart-option";

const number = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 3 });
const axisNumber = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 2 });

/**
 * Gráfico de uma resposta do assistente: linha no tempo ou barras por
 * categoria. A primeira série na cor de destaque, a segunda em cinza (mesma
 * regra das outras páginas: duas cores, legenda e tabela ao lado).
 */
export function buildAssistantOption(data: ChartData, title: string, colors: ChartColors): EChartsOption {
  const palette = [colors.series, colors.muted];
  const isTime = data.kind === "line" && data.xType === "date";
  const label = (x: string) => (data.xType === "date" ? formatDateAs(x, data.granularity) : x);

  return {
    animation: false,
    textStyle: { fontFamily: colors.font },
    aria: {
      enabled: true,
      label: {
        description:
          `${title}: ${data.series.map((s) => s.name).join(" e ")}, ` +
          `${data.x.length} pontos de ${label(data.x[0])} a ${label(data.x[data.x.length - 1])}.`,
      },
    },
    grid: { left: 4, right: 16, top: 20, bottom: 4, containLabel: true },
    xAxis: isTime
      ? {
          type: "time",
          axisLine: { lineStyle: { color: colors.axis } },
          axisTick: { show: false },
          splitLine: { show: false },
          // Rótulos em português e na granularidade dos dados ("set/2026",
          // "2015"), não no formato padrão do ECharts, que é em inglês.
          axisLabel: {
            color: colors.muted,
            hideOverlap: true,
            formatter: (ts: number) => {
              const d = new Date(ts);
              const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
              return formatDateAs(iso, data.granularity === "day" ? "month" : data.granularity);
            },
          },
        }
      : {
          type: "category",
          data: data.x.map(label),
          axisLine: { lineStyle: { color: colors.axis } },
          axisTick: { show: false },
          axisLabel: { color: colors.ink2, hideOverlap: true },
        },
    yAxis: {
      type: "value",
      scale: data.kind === "line", // linha não precisa partir do zero; barra sim
      splitNumber: 4,
      axisLabel: { color: colors.muted, formatter: (v: number) => axisNumber.format(v) },
      splitLine: { lineStyle: { color: colors.grid, width: 1, type: "solid" } },
    },
    tooltip: {
      trigger: "axis",
      axisPointer:
        data.kind === "bar"
          ? { type: "shadow", shadowStyle: { color: colors.hover } }
          : { type: "line", lineStyle: { color: colors.axis, width: 1 } },
      backgroundColor: colors.surface,
      borderColor: colors.hairline,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: colors.ink, fontFamily: colors.font, fontSize: 13 },
      extraCssText: "border-radius: 8px; box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);",
      formatter: (params) => {
        const list = (Array.isArray(params) ? params : [params]) as unknown as {
          seriesIndex: number;
          dataIndex: number;
        }[];
        const i = list[0]?.dataIndex ?? 0;
        const rows = data.series.map((s) => {
          const v = s.values[i];
          return (
            `<div style="display:flex;align-items:baseline;gap:8px">` +
            `<span style="font-weight:600;font-size:15px">${v === null ? "—" : number.format(v)}</span>` +
            `<span style="color:${colors.ink2}">${s.name}</span></div>`
          );
        });
        return rows.join("") + `<div style="color:${colors.ink2}">${label(data.x[i] ?? "")}</div>`;
      },
    },
    series: data.series.map((s, k) => {
      const color = palette[k];
      const points = isTime ? s.values.map((v, i) => [data.x[i], v]) : s.values;
      return data.kind === "bar"
        ? {
            type: "bar" as const,
            name: s.name,
            data: points,
            barMaxWidth: 32,
            itemStyle: { color, borderRadius: [4, 4, 0, 0] },
            emphasis: { disabled: true },
          }
        : {
            type: "line" as const,
            name: s.name,
            data: points,
            connectNulls: false,
            showSymbol: data.x.length <= 24, // marcadores só em séries curtas
            symbolSize: 6,
            lineStyle: { width: 2, color },
            itemStyle: { color },
            emphasis: { disabled: true },
          };
    }),
  };
}
