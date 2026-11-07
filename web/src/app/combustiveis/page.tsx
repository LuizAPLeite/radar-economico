import type { Metadata } from "next";
import { Suspense } from "react";

import { FuelDashboard, FuelsSkeleton } from "@/components/fuels/FuelDashboard";

export const metadata: Metadata = {
  title: "Combustíveis",
  description:
    "Preço médio semanal de gasolina, etanol, diesel e GNV no Brasil e em cada estado, e o preço por município no mapa.",
};

/**
 * Como na página inicial, só o que depende da API fica dentro do <Suspense>.
 * Os filtros vêm da URL (searchParams), que só existe a cada requisição:
 * a página repassa a Promise sem esperar por ela, e quem a lê (await) é o
 * FuelDashboard, já dentro do <Suspense>. Assim o resto continua estático.
 */
export default function FuelsPage({ searchParams }: PageProps<"/combustiveis">) {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col px-4 py-10 sm:py-12">
      <header className="mb-8">
        <h1 className="text-3xl font-semibold tracking-tight text-ink">Preços dos combustíveis</h1>
        <p className="mt-2 max-w-2xl text-ink-2">
          O preço médio nas bombas, da pesquisa semanal da ANP em milhares de postos: no Brasil, em cada estado e em
          cada município pesquisado.
        </p>
      </header>

      <main className="flex-1">
        <Suspense fallback={<FuelsSkeleton />}>
          <FuelDashboard searchParams={searchParams} />
        </Suspense>
      </main>

      <footer className="mt-12 border-t border-hairline pt-6 text-sm text-ink-2">
        Fontes: ANP, Levantamento de Preços de Combustíveis (série histórica de preços de revenda); IBGE, municípios
        e malha dos estados. Código aberto no{" "}
        <a
          className="text-ink underline underline-offset-4 hover:no-underline"
          href="https://github.com/LuizAPLeite/radar-economico"
        >
          GitHub
        </a>
        .
      </footer>
    </div>
  );
}
