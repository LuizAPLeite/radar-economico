import type { WeekPrice } from "@/lib/api";
import { compareWithNational, comparisonText, fillWeeklyGaps, formatPrice } from "@/lib/fuels";
import { formatDate } from "@/lib/indicators";
import { WeeklyChart } from "./WeeklyChart";
import type { WeeklySeries } from "./weekly-chart-option";

const TABLE_ROWS = 12;

interface Props {
  fuelName: string;
  unit: string;
  national: WeekPrice[];
  state: { uf: string; name: string; points: WeekPrice[] } | null;
}

/** Preço médio semanal: o Brasil e, com uma UF escolhida, a UF em destaque. */
export function WeeklyChartPanel({ fuelName, unit, national, state }: Props) {
  const withState = state !== null && state.points.length > 0;
  const series: WeeklySeries[] = [
    ...(withState
      ? [{ id: state.uf, name: state.name, shortName: state.uf, points: fillWeeklyGaps(state.points), emphasis: true }]
      : []),
    { id: "BR", name: "Brasil", shortName: "Brasil", points: fillWeeklyGaps(national), emphasis: !withState },
  ];
  const comparison = withState ? compareWithNational(state.points, national) : null;
  const hasGaps = series.some((s) => s.points.some(([, price]) => price === null));

  // Tabela: as semanas mais recentes do Brasil, com a UF ao lado.
  const statePrice = new Map(state?.points.map((p) => [p.week_start, p.avg_price]));
  const rows = national.slice(-TABLE_ROWS).reverse();

  return (
    <figure className="mt-10 rounded-xl border border-hairline bg-surface p-5">
      <figcaption className="mb-3">
        <div className="flex items-baseline justify-between gap-4">
          <h2 className="text-base font-semibold text-ink">{fuelName}: preço médio por semana</h2>
          <span className="text-sm text-ink-2">{unit}</span>
        </div>
        {comparison && (
          <p className="mt-1 text-sm text-ink-2">
            {state?.name}: {comparisonText(comparison)}.
          </p>
        )}
        {state && !withState && (
          <p className="mt-1 text-sm text-ink-2">
            A ANP não pesquisou {fuelName.toLowerCase()} em {state.name} nos últimos 12 meses; o gráfico mostra o
            Brasil.
          </p>
        )}
      </figcaption>

      {series.length > 1 && (
        <ul aria-label="Legenda" className="mb-2 flex flex-wrap gap-x-5 gap-y-1 text-sm text-ink-2">
          {series.map((s) => (
            <li key={s.id} className="flex items-center gap-2">
              <span aria-hidden="true" className={"h-0.5 w-4 rounded-full " + (s.emphasis ? "bg-series" : "bg-muted")} />
              {s.name}
            </li>
          ))}
        </ul>
      )}

      <WeeklyChart series={series} title={`${fuelName}: preço médio por semana`} />

      {hasGaps && (
        <p className="mt-2 text-xs text-ink-2">
          A linha é interrompida nas semanas sem pesquisa completa da ANP.
        </p>
      )}

      {/* A tabela é o equivalente acessível do gráfico: os valores sem depender do mouse. */}
      <details className="mt-3 text-sm">
        <summary className="cursor-pointer text-ink-2 hover:text-ink">
          Ver as {TABLE_ROWS} semanas mais recentes em tabela
        </summary>
        <table className="mt-3 w-full max-w-md tabular-nums">
          <thead>
            <tr className="border-b border-hairline text-left text-ink-2">
              <th className="py-1.5 font-medium">Semana de</th>
              {withState && <th className="py-1.5 text-right font-medium">{state.name}</th>}
              <th className="py-1.5 text-right font-medium">Brasil</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => {
              const uf = statePrice.get(p.week_start);
              return (
                <tr key={p.week_start} className="border-b border-hairline last:border-0">
                  <td className="py-1.5 text-ink-2">{formatDate(p.week_start)}</td>
                  {withState && (
                    <td className="py-1.5 text-right text-ink">{uf === undefined ? "—" : formatPrice(uf)}</td>
                  )}
                  <td className="py-1.5 text-right text-ink">{formatPrice(p.avg_price)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </details>
    </figure>
  );
}
