import Link from "next/link";
import type { ReactNode } from "react";

import { getForecast, getPassThrough, type Driver } from "@/lib/api";
import {
  ANALYSIS_FUELS,
  analysisHref,
  bestByRegime,
  currentRegime,
  DRIVER_LABELS,
  formatCorrelation,
  formatLag,
  lagProfile,
  MODEL_LABELS,
  parseAnalysisFuel,
  regimeShortName,
  rollingSeries,
  summarizeBacktests,
} from "@/lib/analysis";
import { fillWeeklyGaps, formatPrice, type SearchParams } from "@/lib/fuels";
import { formatDate, formatNumber } from "@/lib/indicators";
import { CHART_HEIGHT } from "../chart-option";
import { chipClass } from "../controls";
import { AnalysisChart } from "./AnalysisChart";

const MAIN: Driver = "brent_brl";
const REFERENCE: Driver = "usd_brl";

/**
 * Parte da página de análises que depende da API. Server Component: busca as
 * correlações e a previsão do combustível da URL e monta os textos; os
 * gráficos (no navegador) recebem só os dados.
 */
export async function AnalysisDashboard({ searchParams }: { searchParams: Promise<SearchParams> }) {
  const fuelId = parseAnalysisFuel(await searchParams);
  const [pass, forecast] = await Promise.all([getPassThrough(fuelId), getForecast(fuelId)]);
  const fuelInfo = ANALYSIS_FUELS.find((f) => f.id === fuelId)!; // parseAnalysisFuel só devolve ids da lista
  const fuelName = fuelInfo.label;

  if (pass.lags.length === 0 || pass.regimes.length === 0) {
    return (
      <>
        <FuelLinks selected={fuelId} />
        <EmptyState />
      </>
    );
  }

  const current = currentRegime(pass.regimes)!;
  const mainBest = bestByRegime(pass.lags, MAIN, pass.regimes);
  const refBest = bestByRegime(pass.lags, REFERENCE, pass.regimes);
  const nowMain = mainBest.find((b) => b.regime.id === current.id);
  const nowRef = refBest.find((b) => b.regime.id === current.id);
  const regimeNames = pass.regimes.map(regimeShortName);
  const byRegime = (best: typeof mainBest) =>
    pass.regimes.map((r) => best.find((b) => b.regime.id === r.id) ?? null);

  const mainProfile = lagProfile(pass.lags, MAIN, current.id);
  const refProfile = lagProfile(pass.lags, REFERENCE, current.id);
  const mainRolling = rollingSeries(pass.rolling, MAIN);
  const refRolling = rollingSeries(pass.rolling, REFERENCE);
  const yearEnds = lastOfEachYear(mainRolling, refRolling);

  const backtest = summarizeBacktests(forecast.backtests);
  const next = forecast.forecasts[0];
  const lastForecast = forecast.forecasts.at(-1);

  return (
    <>
      <FuelLinks selected={fuelId} />

      {nowMain && (
        <p className="mt-6 max-w-3xl text-lg text-ink">
          {regimeShortName(current)}, quando o petróleo em reais sobe, o preço {fuelInfo.of} na bomba tende a subir{" "}
          {formatLag(nowMain.lag)} (correlação de {formatCorrelation(nowMain.correlation)}).
          {nowRef && <> O dólar sozinho explica pouco ({formatCorrelation(nowRef.correlation)}).</>}
        </p>
      )}
      <p className="mt-2 max-w-3xl text-sm text-ink-2">
        Correlação entre a variação em 4 semanas do preço médio no Brasil e a do fator algumas semanas antes: 1 seria
        acompanhar sempre, 0 é nenhuma relação, e números negativos, direções opostas. O petróleo em reais é o Brent
        convertido pelo dólar do dia, o custo que a Petrobras compara com o preço de importação.
      </p>

      <Panel
        title="Quanto a bomba acompanha cada fator, por período"
        subtitle="A maior correlação de cada fator em cada período da política de preços da Petrobras."
        legend
      >
        <AnalysisChart
          kind="regimes"
          title={`${fuelName}: correlação com o petróleo e com o dólar, por período`}
          data={{
            regimes: regimeNames,
            main: {
              name: DRIVER_LABELS[MAIN],
              values: byRegime(mainBest).map((b) => b?.correlation ?? null),
              lags: byRegime(mainBest).map((b) => b?.lag ?? null),
            },
            reference: {
              name: DRIVER_LABELS[REFERENCE],
              values: byRegime(refBest).map((b) => b?.correlation ?? null),
              lags: byRegime(refBest).map((b) => b?.lag ?? null),
            },
          }}
        />
        <ul className="mt-3 grid gap-2 text-sm text-ink-2 sm:grid-cols-3">
          {pass.regimes.map((r) => (
            <li key={r.id}>
              <span className="font-medium text-ink">{regimeShortName(r)}:</span> {r.description}
            </li>
          ))}
        </ul>
        <DataTable
          summary="Ver os valores em tabela"
          head={["Período", DRIVER_LABELS[MAIN], DRIVER_LABELS[REFERENCE]]}
          rows={pass.regimes.map((r, i) => {
            const m = byRegime(mainBest)[i];
            const ref = byRegime(refBest)[i];
            return [
              r.name,
              m ? `${formatCorrelation(m.correlation)} (${formatLag(m.lag)})` : "—",
              ref ? `${formatCorrelation(ref.correlation)} (${formatLag(ref.lag)})` : "—",
            ];
          })}
        />
      </Panel>

      <Panel
        title={`Quanto tempo leva para chegar à bomba (${regimeShortName(current).toLowerCase()})`}
        subtitle="Correlação com o fator 0, 1, 2… semanas antes. O pico mostra a defasagem típica."
        legend
      >
        <AnalysisChart
          kind="lags"
          title={`${fuelName}: correlação por semanas de defasagem, ${regimeShortName(current).toLowerCase()}`}
          data={{
            main: { name: DRIVER_LABELS[MAIN], points: mainProfile },
            reference: { name: DRIVER_LABELS[REFERENCE], points: refProfile },
          }}
        />
        <DataTable
          summary="Ver os valores em tabela"
          head={["Defasagem", DRIVER_LABELS[MAIN], DRIVER_LABELS[REFERENCE]]}
          rows={mainProfile.map(([lag, corr]) => {
            const ref = refProfile.find(([l]) => l === lag);
            return [
              `${lag} semana${lag === 1 ? "" : "s"}`,
              formatCorrelation(corr),
              ref ? formatCorrelation(ref[1]) : "—",
            ];
          })}
        />
      </Panel>

      <Panel
        title="Como a relação mudou ao longo do tempo"
        subtitle="Correlação em janelas de 52 semanas, na defasagem típica do período atual. As linhas tracejadas marcam as mudanças de política."
        legend
      >
        <AnalysisChart
          kind="rolling"
          title={`${fuelName}: correlação móvel de 52 semanas`}
          data={{
            main: { name: DRIVER_LABELS[MAIN], points: mainRolling },
            reference: { name: DRIVER_LABELS[REFERENCE], points: refRolling },
            boundaries: pass.regimes.slice(1).map((r) => ({ date: r.starts_on, label: regimeShortName(r) })),
          }}
        />
        <DataTable
          summary="Ver o valor no fim de cada ano"
          head={["Janela até", DRIVER_LABELS[MAIN], DRIVER_LABELS[REFERENCE]]}
          rows={yearEnds.map(([week, m, ref]) => [
            formatDate(week),
            m === null ? "—" : formatCorrelation(m),
            ref === null ? "—" : formatCorrelation(ref),
          ])}
        />
      </Panel>

      <Panel
        title={`Previsão do preço médio ${fuelInfo.of} no Brasil`}
        subtitle={
          next && lastForecast
            ? `Próxima semana: ${formatPrice(next.predicted)} (80% de chance entre ${formatPrice(next.lower_80)} e ${formatPrice(next.upper_80)}). Em ${lastForecast.horizon_weeks} semanas, entre ${formatPrice(lastForecast.lower_80)} e ${formatPrice(lastForecast.upper_80)}.`
            : "Ainda não há previsão calculada para este combustível."
        }
      >
        {forecast.forecasts.length > 0 && (
          <>
            <ul aria-label="Legenda" className="mb-2 flex flex-wrap gap-x-5 gap-y-1 text-sm text-ink-2">
              <li className="flex items-center gap-2">
                <span aria-hidden="true" className="h-0.5 w-4 rounded-full bg-series" />
                Preço médio (ANP)
              </li>
              <li className="flex items-center gap-2">
                <span aria-hidden="true" className="h-0 w-4 border-t-2 border-dashed border-series" />
                Previsão
              </li>
              <li className="flex items-center gap-2">
                <span aria-hidden="true" className="h-3 w-4 rounded-sm bg-series opacity-20" />
                Faixa de 80%
              </li>
            </ul>
            <AnalysisChart
              kind="forecast"
              title={`${fuelName}: preço médio semanal e previsão`}
              data={{
                history: fillWeeklyGaps(forecast.history),
                forecast: forecast.forecasts.map((f) => ({
                  week: f.week_start,
                  predicted: f.predicted,
                  lower: f.lower_80,
                  upper: f.upper_80,
                })),
              }}
            />
          </>
        )}

        {backtest.rows.length > 0 && (
          <div className="mt-5 border-t border-hairline pt-4">
            <h3 className="text-sm font-semibold text-ink">O teste com o passado</h3>
            <p className="mt-1 max-w-3xl text-sm text-ink-2">
              Cada modelo previu, semana a semana, as últimas 104 semanas usando só o que se sabia na época.
              {forecast.model === "naive" && backtest.brentVsNaivePercent !== null && backtest.brentVsNaivePercent > 0 ? (
                <>
                  {" "}
                  A regressão com o petróleo errou em média{" "}
                  {formatNumber(backtest.brentVsNaivePercent, 0)}% a mais que simplesmente repetir o último preço. A
                  bomba segue o petróleo, mas a Petrobras reajusta em degraus e em datas que ninguém sabe antes: saber
                  a direção não diz quando. Por isso a previsão publicada é a do preço que fica igual, com a faixa de
                  incerteza medida no teste.
                </>
              ) : (
                forecast.model && <> O modelo publicado é o de menor erro: {MODEL_LABELS[forecast.model] ?? forecast.model}.</>
              )}
            </p>
            <table className="mt-3 w-full max-w-lg text-sm tabular-nums">
              <caption className="sr-only">Erro absoluto médio de cada modelo, por semanas à frente</caption>
              <thead>
                <tr className="border-b border-hairline text-left text-ink-2">
                  <th className="py-1.5 font-medium">Semanas à frente</th>
                  <th className="py-1.5 text-right font-medium">{MODEL_LABELS.naive}</th>
                  <th className="py-1.5 text-right font-medium">{MODEL_LABELS.brent}</th>
                </tr>
              </thead>
              <tbody>
                {backtest.rows.map((row) => (
                  <tr key={row.horizon} className="border-b border-hairline last:border-0">
                    <td className="py-1.5 text-ink-2">{row.horizon}</td>
                    <ErrorCell value={row.naive} best={isBest(row.naive, row.brent)} />
                    <ErrorCell value={row.brent} best={isBest(row.brent, row.naive)} />
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="mt-2 text-xs text-ink-2">Erro médio em reais por {unitWord(pass.fuel.unit)}; o menor de cada linha em negrito.</p>
          </div>
        )}

        {forecast.forecasts.length > 0 && (
          <DataTable
            summary="Ver a previsão em tabela"
            head={["Semana de", "Previsão", "Faixa de 80%"]}
            rows={forecast.forecasts.map((f) => [
              formatDate(f.week_start),
              formatPrice(f.predicted),
              `${formatPrice(f.lower_80)} a ${formatPrice(f.upper_80)}`,
            ])}
          />
        )}
      </Panel>
    </>
  );
}

function isBest(value: number | null, other: number | null): boolean {
  return value !== null && (other === null || value <= other);
}

function ErrorCell({ value, best }: { value: number | null; best: boolean }) {
  return (
    <td className={"py-1.5 text-right " + (best ? "font-semibold text-ink" : "text-ink-2")}>
      {value === null ? "—" : `R$ ${formatNumber(value, 3)}`}
    </td>
  );
}

function unitWord(unit: string): string {
  return unit === "R$/l" ? "litro" : unit.replace("R$/", "");
}

/** A última semana de cada ano da correlação móvel, com os dois fatores. */
function lastOfEachYear(
  main: [string, number | null][],
  reference: [string, number | null][],
): [string, number | null, number | null][] {
  const ref = new Map(reference);
  const byYear = new Map<string, [string, number | null]>();
  for (const point of main) {
    if (point[1] !== null) byYear.set(point[0].slice(0, 4), point);
  }
  return [...byYear.values()].map(([week, value]) => [week, value, ref.get(week) ?? null]);
}

function FuelLinks({ selected }: { selected: string }) {
  return (
    <nav aria-label="Combustível" className="flex flex-wrap gap-2">
      {ANALYSIS_FUELS.map((f) => (
        <Link
          key={f.id}
          href={analysisHref(f.id)}
          scroll={false}
          aria-current={f.id === selected ? "page" : undefined}
          className={chipClass(f.id === selected)}
        >
          {f.label}
        </Link>
      ))}
    </nav>
  );
}

function Panel({
  title,
  subtitle,
  legend = false,
  children,
}: {
  title: string;
  subtitle: string;
  legend?: boolean;
  children: ReactNode;
}) {
  return (
    <figure className="mt-8 rounded-xl border border-hairline bg-surface p-5">
      <figcaption className="mb-3">
        <h2 className="text-base font-semibold text-ink">{title}</h2>
        <p className="mt-1 text-sm text-ink-2">{subtitle}</p>
      </figcaption>
      {legend && (
        <ul aria-label="Legenda" className="mb-2 flex flex-wrap gap-x-5 gap-y-1 text-sm text-ink-2">
          <li className="flex items-center gap-2">
            <span aria-hidden="true" className="h-2.5 w-2.5 rounded-sm bg-series" />
            {DRIVER_LABELS[MAIN]}
          </li>
          <li className="flex items-center gap-2">
            <span aria-hidden="true" className="h-2.5 w-2.5 rounded-sm bg-muted" />
            {DRIVER_LABELS[REFERENCE]}
          </li>
        </ul>
      )}
      {children}
    </figure>
  );
}

/** O equivalente acessível de cada gráfico: os valores sem depender do mouse. */
function DataTable({ summary, head, rows }: { summary: string; head: string[]; rows: string[][] }) {
  return (
    <details className="mt-3 text-sm">
      <summary className="cursor-pointer text-ink-2 hover:text-ink">{summary}</summary>
      <table className="mt-3 w-full max-w-lg tabular-nums">
        <thead>
          <tr className="border-b border-hairline text-left text-ink-2">
            {head.map((h, i) => (
              <th key={h} className={"py-1.5 font-medium " + (i > 0 ? "text-right" : "")}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row[0]} className="border-b border-hairline last:border-0">
              {row.map((cell, i) => (
                <td key={i} className={"py-1.5 " + (i > 0 ? "text-right text-ink" : "text-ink-2")}>
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </details>
  );
}

function EmptyState() {
  return (
    <div className="mt-6 rounded-xl border border-hairline bg-surface p-6">
      <h2 className="font-semibold text-ink">Ainda não há análises</h2>
      <p className="mt-2 text-sm text-ink-2">
        A API está no ar, mas o job de análise ainda não rodou. Colete a série da ANP e o Brent e rode o pipeline:{" "}
        <code className="font-mono text-ink">docker compose up collector pipeline</code>
      </p>
    </div>
  );
}

/** Mostrado enquanto os dados chegam: mesmo tamanho do conteúdo, para a página não pular. */
export function AnalysisSkeleton() {
  const block = "animate-pulse rounded-xl border border-hairline bg-surface";
  return (
    <div aria-busy="true">
      <p role="status" className="sr-only">
        Carregando as análises…
      </p>
      <div className="h-8 w-full max-w-md animate-pulse rounded-full bg-hover" />
      <div className="mt-6 h-16 max-w-3xl animate-pulse rounded-lg bg-hover" />
      {[1, 2, 3, 4].map((i) => (
        <div key={i} className={`mt-8 ${block}`} style={{ height: CHART_HEIGHT + 120 }} />
      ))}
    </div>
  );
}
