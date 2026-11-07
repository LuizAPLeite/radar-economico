import { getFuelMunicipalities, getFuelPrices, listFuels } from "@/lib/api";
import {
  fuelLabel,
  mapScale,
  municipalityCard,
  parseFuelFilters,
  priceCard,
  rankMunicipalities,
  sortFuels,
  UF_NAMES,
  type SearchParams,
} from "@/lib/fuels";
import { formatUpdatedAt } from "@/lib/indicators";
import { CHART_HEIGHT } from "../chart-option";
import { IndicatorCard } from "../IndicatorCard";
import { FuelControls, FuelFiltersProvider, PendingArea } from "./FuelFilters";
import { FuelMapPanel } from "./FuelMapPanel";
import { MunicipalTable } from "./MunicipalTable";
import { Rankings } from "./Rankings";
import { MAP_HEIGHT_CLASS } from "./sizes";
import { WeeklyChartPanel } from "./WeeklyChartPanel";

const RANKING_SIZE = 5;

/**
 * Parte da página de combustíveis que depende da API e dos filtros da URL.
 * Server Component: a cada troca de filtro, roda de novo no servidor do Next,
 * que busca os dados na API e manda o resultado pronto ao navegador.
 */
export async function FuelDashboard({ searchParams }: { searchParams: Promise<SearchParams> }) {
  const [params, fuels] = await Promise.all([searchParams, listFuels().then(sortFuels)]);
  if (fuels.length === 0) {
    return <EmptyState />;
  }

  const filters = parseFuelFilters(params, fuels.map((f) => f.id));
  const fuel = fuels.find((f) => f.id === filters.fuel) ?? fuels[0];

  // As três consultas não dependem umas das outras: vão ao mesmo tempo.
  const [national, statePoints, requested] = await Promise.all([
    getFuelPrices(fuel.id, "BR"),
    filters.uf ? getFuelPrices(fuel.id, filters.uf) : null,
    getFuelMunicipalities(fuel.id, filters.month),
  ]);
  // Um mês sem dados (um link antigo, por exemplo) cai no mais recente.
  const unknownMonth = filters.month !== null && !requested.available_months.includes(filters.month);
  const municipal = unknownMonth ? await getFuelMunicipalities(fuel.id, null) : requested;

  const label = fuelLabel(fuel);
  const ufName = filters.uf ? UF_NAMES[filters.uf] : null;
  const inScope = filters.uf ? municipal.data.filter((m) => m.uf === filters.uf) : municipal.data;
  const ranking = rankMunicipalities(inScope, RANKING_SIZE);
  const cards = [
    priceCard(`${label} · ${ufName ?? "Brasil"}`, fuel.unit, statePoints ?? national),
    municipalityCard("cheapest", ranking.cheapest[0], municipal.month),
    municipalityCard("priciest", ranking.priciest[0], municipal.month),
  ];

  return (
    <FuelFiltersProvider filters={{ ...filters, month: unknownMonth ? null : filters.month }}>
      <FuelControls fuels={fuels.map((f) => ({ id: f.id, label: fuelLabel(f) }))} />
      <PendingArea>
        <section aria-label="Resumo" className="mt-6 grid gap-4 sm:grid-cols-3">
          {cards.map((card) => (
            <IndicatorCard key={card.id} card={card} />
          ))}
        </section>
        <p className="mt-3 text-sm text-ink-2">Dados atualizados em {formatUpdatedAt(fuel.updated_at)}.</p>

        <WeeklyChartPanel
          fuelName={label}
          unit={fuel.unit}
          national={national}
          state={filters.uf && ufName && statePoints ? { uf: filters.uf, name: ufName, points: statePoints } : null}
        />

        <div className="mt-4 grid gap-4 lg:grid-cols-5">
          <FuelMapPanel
            className="lg:col-span-3"
            // A escala de cores usa o país inteiro: a mesma cor quer dizer o
            // mesmo preço em qualquer estado.
            municipalities={municipal.data}
            scale={mapScale(municipal.data.map((m) => m.avg_price))}
            month={municipal.month}
            availableMonths={municipal.available_months}
            uf={filters.uf}
            unit={fuel.unit}
          />
          <Rankings
            className="lg:col-span-2"
            cheapest={ranking.cheapest}
            priciest={ranking.priciest}
            scopeName={ufName ?? "Brasil"}
            month={municipal.month}
          />
        </div>

        <MunicipalTable data={inScope} month={municipal.month} />
      </PendingArea>
    </FuelFiltersProvider>
  );
}

function EmptyState() {
  return (
    <div className="rounded-xl border border-hairline bg-surface p-6">
      <h2 className="font-semibold text-ink">Ainda não há dados de combustíveis</h2>
      <p className="mt-2 text-sm text-ink-2">
        A API está no ar, mas a mart está vazia. Rode o coletor e o pipeline:{" "}
        <code className="font-mono text-ink">docker compose up collector pipeline</code>
      </p>
    </div>
  );
}

/** Mostrado enquanto os dados chegam: mesmo tamanho do conteúdo, para a página não pular. */
export function FuelsSkeleton() {
  const block = "animate-pulse rounded-xl border border-hairline bg-surface";
  return (
    <div aria-busy="true">
      <p role="status" className="sr-only">
        Carregando os preços dos combustíveis…
      </p>
      <div className="h-8 w-full max-w-xl animate-pulse rounded-full bg-hover" />
      <div className="mt-6 grid gap-4 sm:grid-cols-3">
        {[1, 2, 3].map((i) => (
          <div key={i} className={`h-40 ${block}`} />
        ))}
      </div>
      <div className={`mt-10 ${block}`} style={{ height: CHART_HEIGHT + 120 }} />
      <div className="mt-4 grid gap-4 lg:grid-cols-5">
        <div className={`lg:col-span-3 p-5 ${block}`}>
          <div className={MAP_HEIGHT_CLASS} />
        </div>
        <div className={`h-80 lg:col-span-2 ${block}`} />
      </div>
    </div>
  );
}
