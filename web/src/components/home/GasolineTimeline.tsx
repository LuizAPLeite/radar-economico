import { getFuelHistory } from "@/lib/api";
import { formatBRL, monthlyAverages, yearlyAverages } from "@/lib/history";
import { PriceTimeline } from "./PriceTimeline";

/**
 * Busca a série oficial da gasolina no servidor e entrega ao navegador só as
 * médias mensais. A tabela por ano é o equivalente acessível da linha.
 */
export async function GasolineTimeline() {
  // A linha é a abertura da página, não o conteúdo principal: se a série
  // falhar, a página segue com os indicadores em vez de virar uma tela de erro.
  let weeks: Awaited<ReturnType<typeof getFuelHistory>>;
  try {
    weeks = await getFuelHistory("gasolina");
  } catch {
    return null;
  }
  const months = monthlyAverages(weeks);
  if (months.length < 2) {
    return null;
  }
  const years = yearlyAverages(months);
  return (
    <>
      <PriceTimeline months={months} />
      <div className="mx-auto w-full max-w-5xl px-4">
        <details className="text-sm">
          <summary className="cursor-pointer text-ink-2 hover:text-ink">
            Ver a média de cada ano em tabela
          </summary>
          <table className="mt-3 w-full max-w-xs tabular-nums">
            <thead>
              <tr className="border-b border-hairline text-left text-ink-2">
                <th className="py-1.5 font-medium">Ano</th>
                <th className="py-1.5 text-right font-medium">Média do litro</th>
              </tr>
            </thead>
            <tbody>
              {years.map(({ year, price }) => (
                <tr key={year} className="border-b border-hairline last:border-0">
                  <td className="py-1.5 text-ink-2">{year}</td>
                  <td className="py-1.5 text-right text-ink">{formatBRL(price)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      </div>
    </>
  );
}

/** Mesmo tamanho da primeira tela, para a página não pular quando os dados chegam. */
export function GasolineTimelineSkeleton() {
  return (
    <div aria-busy="true" className="mx-auto flex h-svh w-full max-w-5xl flex-col justify-center px-4">
      <p role="status" className="sr-only">
        Carregando a série da gasolina…
      </p>
      <div className="h-6 w-80 max-w-full animate-pulse rounded-sm bg-hover" />
      <div className="mt-4 h-36 w-96 max-w-full animate-pulse rounded-sm bg-hover" />
      <div className="mt-10 h-[34svh] min-h-44 w-full animate-pulse rounded-sm bg-hover" />
    </div>
  );
}
