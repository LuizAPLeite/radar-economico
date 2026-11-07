"use client";

import { useMemo, useSyncExternalStore } from "react";
import ReactEChartsCore from "echarts-for-react/lib/core";
import * as echarts from "echarts/core";
import { BarChart, LineChart } from "echarts/charts";
import { AriaComponent, GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";

import type { ChartKind } from "@/lib/indicators";
import { buildChartOption, CHART_HEIGHT, readChartColors, type Tuple } from "./chart-option";

// Registra só as partes do ECharts que o dashboard usa: o JavaScript enviado
// ao navegador fica bem menor do que importando a biblioteca inteira.
echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, AriaComponent, CanvasRenderer]);

// Tema claro/escuro do sistema. useSyncExternalStore assina uma fonte externa
// (a media query) e re-renderiza quando ela muda; no servidor não existe tema,
// então ali só reservamos o espaço do gráfico.
const DARK_QUERY = "(prefers-color-scheme: dark)";

function subscribe(onChange: () => void) {
  const media = window.matchMedia(DARK_QUERY);
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}
const getScheme = () => (window.matchMedia(DARK_QUERY).matches ? "dark" : "light");
const getServerScheme = () => null;

interface Props {
  id: string;
  title: string;
  kind: ChartKind;
  points: Tuple[];
  start: string;
  end: string;
}

export function TimeSeriesChart({ id, title, kind, points, start, end }: Props) {
  const scheme = useSyncExternalStore(subscribe, getScheme, getServerScheme);

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
