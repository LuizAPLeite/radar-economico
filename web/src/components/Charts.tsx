"use client";

import { useState } from "react";

import { PERIODS, periodStart, type PeriodId } from "@/lib/indicators";
import { ChartPanel, type ChartSeries } from "./ChartPanel";
import { chipClass } from "./controls";

/**
 * Os gráficos com um seletor de período único: os três mostram sempre a
 * mesma janela de tempo, então dá para comparar um com o outro.
 */
export function Charts({ series, seriesStart }: { series: ChartSeries[]; seriesStart: string }) {
  const [period, setPeriod] = useState<PeriodId>("5y");

  // Todas as janelas terminam na data mais recente entre as séries.
  const end = series
    .map((s) => s.points.at(-1)?.[0] ?? seriesStart)
    .reduce((a, b) => (a > b ? a : b), seriesStart);
  const years = PERIODS.find((p) => p.id === period)?.years ?? null;
  const start = periodStart(end, years, seriesStart);

  return (
    <section aria-label="Séries históricas" className="mt-10">
      <div role="group" aria-label="Período dos gráficos" className="flex flex-wrap items-center gap-2">
        <span className="mr-1 text-sm text-ink-2">Período</span>
        {PERIODS.map((p) => {
          const selected = p.id === period;
          return (
            <button
              key={p.id}
              type="button"
              aria-pressed={selected}
              onClick={() => setPeriod(p.id)}
              className={chipClass(selected)}
            >
              {p.label}
            </button>
          );
        })}
      </div>

      <div className="mt-4 grid gap-4">
        {series.map((s) => (
          <ChartPanel key={s.id} series={s} start={start} end={end} />
        ))}
      </div>
    </section>
  );
}
