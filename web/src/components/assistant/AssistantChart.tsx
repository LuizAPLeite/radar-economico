"use client";

import { useMemo } from "react";
import ReactEChartsCore from "echarts-for-react/lib/core";
import * as echarts from "echarts/core";
import { BarChart, LineChart } from "echarts/charts";
import { AriaComponent, GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

import type { ChartData } from "@/lib/assistant";
import { CHART_HEIGHT, readChartColors } from "../chart-option";
import { useColorScheme } from "../use-color-scheme";
import { buildAssistantOption } from "./assistant-chart-option";

echarts.use([BarChart, LineChart, GridComponent, TooltipComponent, AriaComponent, CanvasRenderer]);

export function AssistantChart({ data, title }: { data: ChartData; title: string }) {
  const scheme = useColorScheme();
  const option = useMemo(
    () => (scheme ? buildAssistantOption(data, title, readChartColors()) : null),
    [scheme, data, title],
  );
  if (!option) {
    return <div style={{ height: CHART_HEIGHT }} aria-hidden="true" />;
  }
  return (
    <ReactEChartsCore echarts={echarts} option={option} notMerge style={{ height: CHART_HEIGHT }} opts={{ renderer: "canvas" }} />
  );
}
