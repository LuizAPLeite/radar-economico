"use client";

import { useRouter } from "next/navigation";
import { createContext, use, useOptimistic, useTransition, type ReactNode } from "react";

import { fuelsHref, UF_NAMES, UFS, type FuelFilters } from "@/lib/fuels";
import { chipClass, SELECT_CLASS } from "../controls";

/*
 * Os filtros moram na URL (?combustivel=etanol&uf=SP&mes=2026-10): a página
 * pode ser compartilhada já filtrada, e o botão Voltar desfaz a última troca.
 * Trocar um filtro é uma navegação: o servidor monta a página de novo com os
 * dados novos, e o navegador continua sem falar com a API.
 *
 * Enquanto a navegação não termina, o useOptimistic já mostra a escolha nova
 * nos controles, e o conteúdo antigo fica esmaecido (`pending`) em vez de
 * voltar para o esqueleto de carregamento.
 */

interface FiltersState {
  filters: FuelFilters;
  pending: boolean;
  change: (next: Partial<FuelFilters>) => void;
}

const FiltersContext = createContext<FiltersState | null>(null);

/** Filtros atuais e a função que troca um deles (para qualquer controle da página). */
export function useFuelFilters(): FiltersState {
  const state = use(FiltersContext);
  if (!state) {
    throw new Error("useFuelFilters precisa de um <FuelFiltersProvider> acima");
  }
  return state;
}

export function FuelFiltersProvider({ filters, children }: { filters: FuelFilters; children: ReactNode }) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [optimistic, setOptimistic] = useOptimistic(filters);

  function change(next: Partial<FuelFilters>) {
    // Parte do valor otimista: duas trocas rápidas seguidas não se desfazem.
    const target = { ...optimistic, ...next };
    startTransition(() => {
      setOptimistic(target);
      router.push(fuelsHref(target), { scroll: false });
    });
  }

  return <FiltersContext value={{ filters: optimistic, pending, change }}>{children}</FiltersContext>;
}

/** Botões de combustível e o seletor de estado. */
export function FuelControls({ fuels }: { fuels: { id: string; label: string }[] }) {
  const { filters, change } = useFuelFilters();
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div role="group" aria-label="Combustível" className="flex flex-wrap gap-2">
        {fuels.map((fuel) => {
          const selected = fuel.id === filters.fuel;
          return (
            <button
              key={fuel.id}
              type="button"
              aria-pressed={selected}
              onClick={() => {
                if (!selected) change({ fuel: fuel.id });
              }}
              className={chipClass(selected)}
            >
              {fuel.label}
            </button>
          );
        })}
      </div>
      <label className="flex items-center gap-2 text-sm text-ink-2">
        Estado
        <select
          value={filters.uf ?? ""}
          onChange={(e) => change({ uf: e.target.value || null })}
          className={SELECT_CLASS}
        >
          <option value="">Brasil (todos)</option>
          {UFS.map((uf) => (
            <option key={uf} value={uf}>
              {UF_NAMES[uf]}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}

/** Esmaece o conteúdo enquanto os dados do filtro novo chegam. */
export function PendingArea({ children }: { children: ReactNode }) {
  const { pending } = useFuelFilters();
  return (
    <>
      {/* Fora da área com aria-busy: leitores de tela adiam o que está dentro dela. */}
      <p role="status" className="sr-only">
        {pending ? "Atualizando os dados…" : ""}
      </p>
      <div
        aria-busy={pending}
        // O atraso evita piscar quando a resposta é rápida.
        className={"transition-opacity duration-200 " + (pending ? "opacity-50 delay-150" : "")}
      >
        {children}
      </div>
    </>
  );
}
