"use client";

import dynamic from "next/dynamic";

import type { MunicipalPrice } from "@/lib/api";
import { formatRange, type MapClass } from "@/lib/fuels";
import { formatMonth, formatNumber } from "@/lib/indicators";
import { SELECT_CLASS } from "../controls";
import { useFuelFilters } from "./FuelFilters";
import { MAP_HEIGHT_CLASS } from "./sizes";

// O MapLibre desenha com WebGL e só existe no navegador: o mapa não passa pelo
// servidor (ssr: false), e o código dele só é baixado quando a página o usa.
const FuelMap = dynamic(() => import("./FuelMap"), {
  ssr: false,
  loading: () => <div aria-hidden="true" className={`${MAP_HEIGHT_CLASS} animate-pulse rounded-lg bg-hover`} />,
});

/** Total de municípios do país (IBGE), para dar a proporção dos pesquisados. */
const MUNICIPALITIES_IN_BRAZIL = 5571;

interface Props {
  municipalities: MunicipalPrice[];
  scale: MapClass[];
  month: string | null; // mês mostrado (AAAA-MM)
  availableMonths: string[]; // em ordem crescente
  uf: string | null;
  unit: string;
  className?: string;
}

export function FuelMapPanel({ municipalities, scale, month, availableMonths, uf, unit, className = "" }: Props) {
  const { filters, change } = useFuelFilters();
  const latest = availableMonths.at(-1) ?? null;
  const selected = filters.month ?? latest;

  return (
    <figure className={`rounded-xl border border-hairline bg-surface p-5 ${className}`}>
      <figcaption className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold text-ink">Preço médio por município</h2>
        {availableMonths.length > 0 && (
          <label className="flex items-center gap-2 text-sm text-ink-2">
            Mês
            <select
              value={selected ?? ""}
              // O mês mais recente é o padrão: não precisa ficar na URL.
              onChange={(e) => change({ month: e.target.value === latest ? null : e.target.value })}
              className={SELECT_CLASS}
            >
              {availableMonths.toReversed().map((m) => (
                <option key={m} value={m}>
                  {formatMonth(m)}
                </option>
              ))}
            </select>
          </label>
        )}
      </figcaption>

      {municipalities.length === 0 || !month ? (
        <p className="text-sm text-ink-2">Ainda não há preços por município para este combustível.</p>
      ) : (
        <>
          <FuelMap municipalities={municipalities} scale={scale} uf={uf} unit={unit} />
          <Legend scale={scale} unit={unit} />
          <p className="mt-3 text-xs text-ink-2">
            Cada ponto é um município pesquisado pela ANP em {formatMonth(month)}:{" "}
            {formatNumber(municipalities.length, 0)} dos {formatNumber(MUNICIPALITIES_IN_BRAZIL, 0)}. Cada cor reúne
            cerca de um quinto deles, do mais barato ao mais caro. Os mesmos dados estão na tabela abaixo.
          </p>
        </>
      )}
    </figure>
  );
}

/** Faixas de preço com a cor de cada uma (as variáveis CSS trocam sozinhas no modo escuro). */
function Legend({ scale, unit }: { scale: MapClass[]; unit: string }) {
  return (
    <div className="mt-3">
      <p className="text-xs text-ink-2">Preço médio no mês ({unit})</p>
      <ol
        className="mt-1 grid gap-1"
        style={{ gridTemplateColumns: `repeat(${scale.length}, minmax(0, 1fr))` }}
      >
        {scale.map((c) => (
          <li key={c.from} className="text-xs tabular-nums text-ink-2">
            <span aria-hidden="true" className="block h-2 rounded-sm" style={{ background: `var(--seq-${c.step})` }} />
            <span className="mt-1 block">{formatRange(c)}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}
