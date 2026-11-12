import type { Metadata } from "next";

import { AssistantPanel } from "@/components/assistant/AssistantPanel";

export const metadata: Metadata = {
  title: "Assistente",
  description:
    "Pergunte em português sobre juros, inflação, dólar, petróleo e combustíveis: um modelo de linguagem escreve a consulta SQL, e a resposta vem com gráfico, tabela e o SQL usado.",
};

/**
 * Página estática: só o painel (Client Component) é interativo, e a pergunta
 * vai ao servidor do Next por uma Server Action (app/assistente/actions.ts).
 */
export default function AssistantPage() {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col px-4 py-10 sm:py-12">
      <header className="mb-8">
        <h1 className="text-3xl font-semibold tracking-tight text-ink">Pergunte aos dados</h1>
        <p className="mt-2 max-w-2xl text-ink-2">
          Faça uma pergunta em português sobre a Selic, o IPCA, o dólar, o petróleo ou o preço dos combustíveis. Um
          modelo de linguagem escreve a consulta SQL, e a resposta vem direto do banco do Radar Econômico.
        </p>
      </header>

      <main className="flex-1">
        <AssistantPanel />
      </main>

      <footer className="mt-12 border-t border-hairline pt-6 text-sm text-ink-2">
        <p>
          Como funciona: o Gemini (Google) transforma a pergunta em SQL; a API confere a consulta e a executa num
          acesso somente leitura, restrito às tabelas públicas do painel, com tempo máximo. O modelo pode errar ao
          interpretar a pergunta: confira o SQL em cada resposta.
        </p>
        <p className="mt-2">
          Código aberto no{" "}
          <a className="text-ink underline underline-offset-4 hover:no-underline" href="https://github.com/LuizAPLeite/radar-economico">
            GitHub
          </a>
          .
        </p>
      </footer>
    </div>
  );
}
