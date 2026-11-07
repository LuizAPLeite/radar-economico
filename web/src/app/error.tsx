"use client"; // telas de erro precisam rodar no navegador (o botão "Tentar de novo")

import { useEffect } from "react";

/**
 * Mostrada quando algo falha ao montar a página, por exemplo a API fora do ar.
 * retry() busca os dados de novo e, se der certo, a página volta ao normal.
 */
export default function ErrorPage({ error, retry }: { error: unknown; retry: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  // Em produção o Next esconde a mensagem do servidor e manda só um código (digest),
  // que aparece nos logs do servidor e ajuda a achar o erro.
  const digest =
    typeof error === "object" && error !== null && "digest" in error ? String(error.digest) : null;

  return (
    <div className="mx-auto w-full max-w-5xl flex-1 px-4 py-10 sm:py-14">
      <h1 className="text-3xl font-semibold tracking-tight text-ink">Radar Econômico</h1>
      <div role="alert" className="mt-8 rounded-xl border border-hairline bg-surface p-6">
        <h2 className="text-lg font-semibold text-ink">Não foi possível carregar os dados</h2>
        <p className="mt-2 text-ink-2">
          A API de indicadores não respondeu. Costuma ser passageiro; tente de novo em instantes.
        </p>
        <button
          type="button"
          onClick={() => retry()}
          className="mt-5 rounded-full bg-ink px-4 py-2 text-sm font-medium text-page hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-series"
        >
          Tentar de novo
        </button>
        {digest && <p className="mt-4 text-xs text-ink-2">Código do erro: {digest}</p>}
      </div>
    </div>
  );
}
