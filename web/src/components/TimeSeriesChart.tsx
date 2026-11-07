"use client";

import { useMemo } from "react";
import ReactEChartsCore from "echarts-for-react/lib/core";
import * as echarts from "echarts/core";
import { BarChart, LineChart } from "echarts/charts";
import { AriaComponent, GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

import type { ChartKind } from "@/lib/indicators";
import { buildChartOption, CHART_HEIGHT, readChartColors, type Tuple } from "./chart-option";
import { useColorScheme } from "./use-color-scheme";

// Registra só as partes do ECharts que o dashboard usa: o JavaScript enviado
// ao navegador fica bem menor do que importando a biblioteca inteira.
echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, AriaComponent, CanvasRenderer]);

interface Props {
  id: string;
  title: string;
  kind: ChartKind;
  points: Tuple[];
  start: string;
  end: string;
}

export function TimeSeriesChart({ id, title, kind, points, start, end }: Props) {
  const scheme = useColorScheme();

  const option = useMemo(
    () => (scheme ? buildChartOption({ id, title, kind, points, start, end, colors: readChartColors() }) : null),
    [scheme, id, title, kind, points, start, end],
  );

  if (!option) {
    return <div style={{ height: CHART_HEIGHT }} aria-hidden="true" />;
  }
  return (
    <ReactEChartsCore
      echarts={echarts}
      option={option}
      notMerge
      style={{ height: CHART_HEIGHT }}
      opts={{ renderer: "canvas" }}
    />
  );
}
