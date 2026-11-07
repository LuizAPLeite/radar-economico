import { getSeries, listIndicators, SERIES_START, type IndicatorWithLatest } from "@/lib/api";
import { buildCard, formatUpdatedAt } from "@/lib/indicators";
import { CHART_HEIGHT } from "./chart-option";
import { Charts } from "./Charts";
import type { ChartSeries } from "./ChartPanel";
import { IndicatorCard } from "./IndicatorCard";

// Ordem de exibição. Um indicador novo que a API passar a ter entra no fim,
// com o formato genérico.
const DISPLAY_ORDER = ["selic", "ipca", "usd_brl"];

function rank(indicator: IndicatorWithLatest): number {
  const i = DISPLAY_ORDER.indexOf(indicator.id);
  return i === -1 ? DISPLAY_ORDER.length : i;
}

/**
 * Parte da página que depende da API. É um Server Component assíncrono: roda
 * no servidor do Next a cada requisição, e o navegador recebe o HTML pronto.
 */
export async function Dashboard() {
  const indicators = (await listIndicators()).toSorted((a, b) => rank(a) - rank(b));
  if (indicators.length === 0) {
    return <EmptyState />;
  }

  // As séries não dependem umas das outras: busca todas ao mesmo tempo.
  const series = await Promise.all(indicators.map((i) => getSeries(i.id)));

  const updatedAt = indicators.map((i) => i.updated_at).toSorted().at(-1);
  const charts: ChartSeries[] = indicators.map((indicator, k) => ({
    id: indicator.id,
    name: indicator.name,
    unit: indicator.unit,
    // Tuplas [data, valor] ocupam bem menos que objetos na página enviada ao navegador.
    points: series[k].map((p) => [p.date, p.value]),
  }));

  return (
    <>
      <section aria-label="Valores mais recentes" className="grid gap-4 sm:grid-cols-3">
        {indicators.map((indicator, k) => (
          <IndicatorCard key={indicator.id} card={buildCard(indicator, series[k])} />
        ))}
      </section>
      {updatedAt && (
        <p className="mt-3 text-sm text-ink-2">Dados atualizados em {formatUpdatedAt(updatedAt)}.</p>
      )}
      <Charts series={charts} seriesStart={SERIES_START} />
    </>
  );
}

function EmptyState() {
  return (
    <div className="rounded-xl border border-hairline bg-surface p-6">
      <h2 className="font-semibold text-ink">Ainda não há dados</h2>
      <p className="mt-2 text-sm text-ink-2">
        A API está no ar, mas a mart está vazia. Rode o coletor e o pipeline:{" "}
        <code className="font-mono text-ink">docker compose up collector pipeline</code>
      </p>
    </div>
  );
}

/** Mostrado enquanto os dados chegam: mesmo tamanho do conteúdo, para a página não pular. */
export function DashboardSkeleton() {
  return (
    <div aria-busy="true">
      <p role="status" className="sr-only">
        Carregando os dados do Banco Central…
      </p>
      <div className="grid gap-4 sm:grid-cols-3">
        {DISPLAY_ORDER.map((id) => (
          <div key={id} className="h-40 animate-pulse rounded-xl border border-hairline bg-surface" />
        ))}
      </div>
      <div className="mt-10 grid gap-4">
        {DISPLAY_ORDER.map((id) => (
          <div
            key={id}
            className="animate-pulse rounded-xl border border-hairline bg-surface"
            style={{ height: CHART_HEIGHT + 100 }}
          />
        ))}
      </div>
    </div>
  );
}
