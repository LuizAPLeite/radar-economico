"use client";

import { useMemo } from "react";
import ReactEChartsCore from "echarts-for-react/lib/core";
import * as echarts from "echarts/core";
import { BarChart, LineChart } from "echarts/charts";
import { AriaComponent, GridComponent, MarkLineComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

import { CHART_HEIGHT, readChartColors } from "../chart-option";
import { useColorScheme } from "../use-color-scheme";
import {
  buildForecastOption,
  buildLagOption,
  buildRegimeBarsOption,
  buildRollingOption,
  type ForecastData,
  type LagData,
  type RegimeBarsData,
  type RollingData,
} from "./analysis-chart-option";

echarts.use([BarChart, LineChart, GridComponent, TooltipComponent, MarkLineComponent, AriaComponent, CanvasRenderer]);

/*
 * As opções do ECharts têm funções (formatadores), que não atravessam a
 * fronteira servidor → cliente. Por isso o servidor manda só os dados, e as
 * opções são montadas aqui, no navegador, com as cores do tema atual.
 */
type Props = { title: string } & (
  | { kind: "regimes"; data: RegimeBarsData }
  | { kind: "lags"; data: LagData }
  | { kind: "rolling"; data: RollingData }
  | { kind: "forecast"; data: ForecastData }
);

export function AnalysisChart(props: Props) {
  const scheme = useColorScheme();
  const option = useMemo(() => {
    if (!scheme) return null;
    const colors = readChartColors();
    switch (props.kind) {
      case "regimes":
        return buildRegimeBarsOption(props.data, props.title, colors);
      case "lags":
        return buildLagOption(props.data, props.title, colors);
      case "rolling":
        return buildRollingOption(props.data, props.title, colors);
      case "forecast":
        return buildForecastOption(props.data, props.title, colors);
    }
  }, [scheme, props]);

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
