"use client";

import { useMemo } from "react";
import ReactEChartsCore from "echarts-for-react/lib/core";
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import { AriaComponent, GridComponent, TooltipComponent } from "echarts/components";
import { LabelLayout } from "echarts/features";
import { CanvasRenderer } from "echarts/renderers";

import { CHART_HEIGHT, readChartColors } from "../chart-option";
import { useColorScheme } from "../use-color-scheme";
import { buildWeeklyOption, type WeeklySeries } from "./weekly-chart-option";

// LabelLayout é o que afasta os rótulos do fim das linhas quando ficam próximos.
echarts.use([LineChart, GridComponent, TooltipComponent, AriaComponent, LabelLayout, CanvasRenderer]);

export function WeeklyChart({ series, title }: { series: WeeklySeries[]; title: string }) {
  const scheme = useColorScheme();
  const option = useMemo(
    () => (scheme ? buildWeeklyOption(series, title, readChartColors()) : null),
    [scheme, series, title],
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
