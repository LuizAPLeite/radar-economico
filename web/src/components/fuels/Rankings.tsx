import type { MunicipalPrice } from "@/lib/api";
import { formatPrice, MIN_STATIONS_FOR_RANKING } from "@/lib/fuels";
import { formatMonth } from "@/lib/indicators";

interface Props {
  cheapest: MunicipalPrice[];
  priciest: MunicipalPrice[];
  scopeName: string; // "Brasil" ou o nome da UF
  month: string | null;
  className?: string;
}

/** Os municípios mais baratos e os mais caros do mês, no recorte escolhido. */
export function Rankings({ cheapest, priciest, scopeName, month, className = "" }: Props) {
  return (
    <section aria-labelledby="rankings-title" className={`rounded-xl border border-hairline bg-surface p-5 ${className}`}>
      <h2 id="rankings-title" className="text-base font-semibold text-ink">
        Mais baratos e mais caros
      </h2>
      <p className="mt-1 text-sm text-ink-2">
        {scopeName}
        {month && `, ${formatMonth(month)}`}. Só entram municípios com {MIN_STATIONS_FOR_RANKING} ou mais postos
        pesquisados.
      </p>
      {cheapest.length === 0 ? (
        <p className="mt-4 text-sm text-ink-2">Poucos municípios pesquisados para comparar.</p>
      ) : (
        <div className="mt-4 grid gap-5 sm:grid-cols-2 lg:grid-cols-1">
          <RankList title="Mais baratos" items={cheapest} />
          <RankList title="Mais caros" items={priciest} />
        </div>
      )}
    </section>
  );
}

function RankList({ title, items }: { title: string; items: MunicipalPrice[] }) {
  return (
    <div>
      <h3 className="text-sm font-medium text-ink-2">{title}</h3>
      <ol className="mt-1.5 text-sm">
        {items.map((m, i) => (
          <li
            key={m.ibge_code}
            className="flex items-baseline gap-3 border-b border-hairline py-1.5 last:border-0"
          >
            <span className="w-4 shrink-0 text-right tabular-nums text-ink-2">{i + 1}</span>
            <span className="min-w-0 flex-1 truncate text-ink">
              {m.name} <span className="text-ink-2">({m.uf})</span>
            </span>
            <span className="tabular-nums font-medium text-ink">{formatPrice(m.avg_price)}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}
