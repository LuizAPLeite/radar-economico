import { Suspense } from "react";

import { Dashboard, DashboardSkeleton } from "@/components/Dashboard";

/**
 * O cabeçalho e o rodapé são estáticos e entram no HTML gerado no build.
 * O <Suspense> marca a parte que depende da API: no build ela vira o
 * esqueleto (fallback), e os dados entram a cada requisição. Por isso o
 * `next build` não precisa da API no ar.
 */
export default function Home() {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col px-4 py-10 sm:py-12">
      <header className="mb-8">
        <h1 className="text-3xl font-semibold tracking-tight text-ink">
          Selic, inflação, dólar e petróleo
        </h1>
        <p className="mt-2 max-w-2xl text-ink-2">
          Os indicadores do Banco Central e o petróleo Brent (FRED): coletados, validados e servidos
          por um pipeline de dados próprio.
        </p>
      </header>

      <main className="flex-1">
        <Suspense fallback={<DashboardSkeleton />}>
          <Dashboard />
        </Suspense>
      </main>

      <footer className="mt-12 border-t border-hairline pt-6 text-sm text-ink-2">
        Fontes: Banco Central do Brasil, Sistema Gerenciador de Séries Temporais (SGS); petróleo
        Brent da EIA, via FRED (Federal Reserve Bank of St. Louis). Código aberto no{" "}
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
