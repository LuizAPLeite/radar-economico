import { Suspense } from "react";

import { Dashboard, DashboardSkeleton } from "@/components/Dashboard";
import { GasolineTimeline, GasolineTimelineSkeleton } from "@/components/home/GasolineTimeline";

/**
 * Página inicial: a linha do tempo da gasolina (rolar é andar no tempo) e,
 * depois, os indicadores. As partes que dependem da API ficam em <Suspense>:
 * no build viram o esqueleto, e os dados entram a cada requisição. Por isso o
 * `next build` não precisa da API no ar.
 */
export default function Home() {
  return (
    <main className="flex flex-1 flex-col">
      <Suspense fallback={<GasolineTimelineSkeleton />}>
        <GasolineTimeline />
      </Suspense>

      <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col px-4 pt-20 pb-10">
        <header className="mb-8 border-t-2 border-ink pt-6">
          <h2 className="text-3xl font-semibold tracking-tight text-ink">Juros, inflação, dólar e petróleo</h2>
          <p className="mt-2 max-w-2xl text-ink-2">
            O que move o preço na bomba e no resto da economia: a Selic e o IPCA do Banco Central, o dólar e o
            petróleo Brent, coletados todo dia e validados antes de chegar aqui.
          </p>
        </header>

        <div className="flex-1">
          <Suspense fallback={<DashboardSkeleton />}>
            <Dashboard />
          </Suspense>
        </div>

        <footer className="mt-12 border-t border-hairline pt-6 text-sm text-ink-2">
          Fontes: ANP, Levantamento de Preços de Combustíveis; Banco Central do Brasil, Sistema Gerenciador de
          Séries Temporais (SGS); petróleo Brent da EIA, via FRED (Federal Reserve Bank of St. Louis). Código aberto
          no{" "}
          <a
            className="text-ink underline underline-offset-4 hover:no-underline"
            href="https://github.com/LuizAPLeite/radar-economico"
          >
            GitHub
          </a>
          .
        </footer>
      </div>
    </main>
  );
}
