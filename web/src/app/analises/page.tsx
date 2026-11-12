import type { Metadata } from "next";
import { Suspense } from "react";

import { AnalysisDashboard, AnalysisSkeleton } from "@/components/analysis/AnalysisDashboard";

export const metadata: Metadata = {
  title: "Análises",
  description:
    "Quanto o preço da gasolina, do diesel e do etanol acompanha o petróleo e o dólar desde 2004, e a previsão das próximas semanas com o erro medido no passado.",
};

/**
 * Mesmo padrão da página de combustíveis: o combustível vem da URL
 * (searchParams, só disponível a cada requisição) e é lido dentro do
 * <Suspense>, no AnalysisDashboard.
 */
export default function AnalysisPage({ searchParams }: PageProps<"/analises">) {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col px-4 py-10 sm:py-12">
      <header className="mb-8">
        <h1 className="text-3xl font-semibold tracking-tight text-ink">Petróleo, dólar e a bomba</h1>
        <p className="mt-2 max-w-2xl text-ink-2">
          O que move o preço dos combustíveis no Brasil desde 2004, e o que dá para prever: análises calculadas pelo
          pipeline a partir da série oficial da ANP, do dólar do Banco Central e do petróleo Brent.
        </p>
      </header>

      <main className="flex-1">
        <Suspense fallback={<AnalysisSkeleton />}>
          <AnalysisDashboard searchParams={searchParams} />
        </Suspense>
      </main>

      <footer className="mt-12 border-t border-hairline pt-6 text-sm text-ink-2">
        Fontes: ANP, série histórica do Levantamento de Preços de Combustíveis (média semanal no Brasil); Banco
        Central do Brasil, dólar comercial (SGS); petróleo Brent da EIA, via FRED (Federal Reserve Bank of St. Louis).
        Períodos: nova política de preços da Petrobras em 14/10/2016 e fim da paridade internacional em 16/05/2023.
        Código aberto no{" "}
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
