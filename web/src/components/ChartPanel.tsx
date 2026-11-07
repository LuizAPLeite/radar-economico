"use client";

import { useMemo } from "react";

import { chartKind, formatDate, formatIndicatorValue, formatMonth } from "@/lib/indicators";
import type { Tuple } from "./chart-option";
import { TimeSeriesChart } from "./TimeSeriesChart";

export interface ChartSeries {
  id: string;
  name: string;
  unit: string;
  points: Tuple[];
}

const TABLE_ROWS = 12;

/** Um indicador: título, gráfico e a tabela com os valores mais recentes. */
export function ChartPanel({ series, start, end }: { series: ChartSeries; start: string; end: string }) {
  const kind = chartKind(series.id);
  // Datas AAAA-MM-DD comparam certo como texto, sem converter para Date.
  const visible = useMemo(
    () => series.points.filter(([date]) => date >= start && date <= end),
    [series.points, start, end],
  );
  const recent = visible.slice(-TABLE_ROWS).reverse();

  return (
    <figure className="rounded-xl border border-hairline bg-surface p-5">
      <figcaption className="mb-3 flex items-baseline justify-between gap-4">
        <h3 className="text-base font-semibold text-ink">{series.name}</h3>
        <span className="text-sm text-ink-2">{series.unit}</span>
      </figcaption>

      <TimeSeriesChart id={series.id} title={series.name} kind={kind} points={visible} start={start} end={end} />

      {/* A tabela é o equivalente acessível do gráfico: os valores sem depender do mouse. */}
      <details className="mt-3 text-sm">
        <summary className="cursor-pointer text-ink-2 hover:text-ink">
          Ver os {TABLE_ROWS} valores mais recentes em tabela
        </summary>
        <table className="mt-3 w-full max-w-sm tabular-nums">
          <thead>
            <tr className="border-b border-hairline text-left text-ink-2">
              <th className="py-1.5 font-medium">{kind === "bar" ? "Mês" : "Data"}</th>
              <th className="py-1.5 text-right font-medium">Valor</th>
            </tr>
          </thead>
          <tbody>
            {recent.map(([date, value]) => (
              <tr key={date} className="border-b border-hairline last:border-0">
                <td className="py-1.5 text-ink-2">{kind === "bar" ? formatMonth(date) : formatDate(date)}</td>
                <td className="py-1.5 text-right text-ink">{formatIndicatorValue(series.id, value)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </figure>
  );
}
