"use client";

import { useActionState, useMemo, useRef, useState } from "react";

import { ask, type AskState } from "@/app/assistente/actions";
import type { Answer } from "@/lib/api";
import { chartData, columnLabel, dateGranularity, EXAMPLES, formatCell, MAX_QUESTION } from "@/lib/assistant";
import { chipClass } from "../controls";
import { AssistantChart } from "./AssistantChart";

const INITIAL: AskState = { question: "", answer: null, error: null };

/** Linhas mostradas na tabela antes de "mostrar todas". */
const TABLE_PREVIEW = 20;

/**
 * Caixa de pergunta, exemplos e resposta. useActionState (React 19) liga o
 * formulário à Server Action: guarda o último estado e diz se há uma pergunta
 * em andamento (pending), sem estado nem fetch escritos à mão.
 */
export function AssistantPanel() {
  const [state, formAction, pending] = useActionState(ask, INITIAL);
  const [text, setText] = useState("");
  const formRef = useRef<HTMLFormElement>(null);

  function askExample(question: string) {
    setText(question);
    // Espera o React pôr o texto no campo antes de enviar o formulário.
    requestAnimationFrame(() => formRef.current?.requestSubmit());
  }

  return (
    <div>
      <form ref={formRef} action={formAction} className="flex flex-col gap-2 sm:flex-row">
        <label htmlFor="question" className="sr-only">
          Sua pergunta
        </label>
        <input
          id="question"
          name="question"
          type="text"
          value={text}
          onChange={(e) => setText(e.target.value)}
          maxLength={MAX_QUESTION}
          required
          minLength={3}
          placeholder="Ex.: Como o preço do diesel variou desde 2020?"
          className={
            "min-w-0 flex-1 rounded-lg border border-hairline bg-surface px-3 py-2 text-ink placeholder:text-muted " +
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-series"
          }
        />
        <button
          type="submit"
          disabled={pending}
          className={
            "rounded-lg bg-ink px-4 py-2 text-sm font-medium text-page transition-opacity disabled:opacity-50 " +
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-series"
          }
        >
          {pending ? "Consultando…" : "Perguntar"}
        </button>
      </form>

      <div className="mt-4">
        <p className="text-sm text-ink-2">Ou experimente:</p>
        <ul className="mt-2 flex flex-wrap gap-2">
          {EXAMPLES.map((q) => (
            <li key={q}>
              <button type="button" disabled={pending} onClick={() => askExample(q)} className={chipClass(false)}>
                {q}
              </button>
            </li>
          ))}
        </ul>
      </div>

      {/* Leitores de tela anunciam o andamento e o resultado. */}
      <p role="status" aria-live="polite" className="sr-only">
        {pending ? "Consultando os dados…" : state.answer ? `Resposta: ${state.answer.title}` : (state.error ?? "")}
      </p>

      <div aria-busy={pending} className={"mt-8 transition-opacity " + (pending ? "opacity-50" : "")}>
        {state.error && (
          <div className="rounded-xl border border-hairline bg-surface p-5 text-ink">
            <p>{state.error}</p>
          </div>
        )}
        {state.answer && <AnswerView answer={state.answer} />}
      </div>
    </div>
  );
}

function AnswerView({ answer }: { answer: Answer }) {
  const chart = useMemo(() => chartData(answer), [answer]);
  // Granularidade de cada coluna de datas: "2015" numa série anual, "set/2026" numa mensal.
  const granularities = useMemo(
    () => answer.columns.map((c, j) => (c.type === "date" ? dateGranularity(answer.rows.map((r) => r[j])) : "day")),
    [answer],
  );
  const [showAll, setShowAll] = useState(false);

  if (!answer.answerable) {
    return (
      <section className="rounded-xl border border-hairline bg-surface p-5">
        <h2 className="text-base font-semibold text-ink">{answer.title}</h2>
        <p className="mt-2 text-ink-2">{answer.explanation}</p>
      </section>
    );
  }

  const rows = showAll ? answer.rows : answer.rows.slice(0, TABLE_PREVIEW);
  return (
    <figure className="rounded-xl border border-hairline bg-surface p-5">
      <figcaption>
        <h2 className="text-base font-semibold text-ink">{answer.title}</h2>
        <p className="mt-1 text-sm text-ink-2">{answer.explanation}</p>
      </figcaption>

      {chart && (
        <div className="mt-4">
          {/* Legenda sempre, mesmo com uma série: o título vem do modelo e
              nem sempre diz qual coluna está no gráfico. */}
          {chart.series.length > 0 && (
            <ul aria-label="Legenda" className="mb-2 flex flex-wrap gap-x-5 gap-y-1 text-sm text-ink-2">
              {chart.series.map((s, k) => (
                <li key={s.name} className="flex items-center gap-2">
                  <span aria-hidden="true" className={"h-2.5 w-2.5 rounded-sm " + (k === 0 ? "bg-series" : "bg-muted")} />
                  {s.name}
                </li>
              ))}
            </ul>
          )}
          <AssistantChart data={chart} title={answer.title} />
          {chart.omitted.length > 0 && (
            <p className="mt-2 text-xs text-ink-2">
              {chart.omitted.join(" e ")} fica de fora do gráfico por estar noutra escala (um eixo só para tudo
              achataria a linha); os valores estão na tabela.
            </p>
          )}
        </div>
      )}

      {answer.rows.length === 0 ? (
        <p className="mt-4 text-sm text-ink-2">A consulta não encontrou nenhum dado.</p>
      ) : (
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm tabular-nums">
            <thead>
              <tr className="border-b border-hairline text-left text-ink-2">
                {answer.columns.map((c) => (
                  <th key={c.name} className={"py-1.5 pr-4 font-medium " + (c.type === "number" ? "text-right" : "")}>
                    {columnLabel(c.name)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, i) => (
                <tr key={i} className="border-b border-hairline last:border-0">
                  {row.map((value, j) => (
                    <td
                      key={j}
                      className={"py-1.5 pr-4 " + (answer.columns[j].type === "number" ? "text-right text-ink" : "text-ink-2")}
                    >
                      {formatCell(value, answer.columns[j].type, granularities[j])}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          {answer.rows.length > TABLE_PREVIEW && !showAll && (
            <button type="button" onClick={() => setShowAll(true)} className="mt-2 text-sm text-ink-2 underline underline-offset-4 hover:text-ink">
              Mostrar as {answer.rows.length} linhas
            </button>
          )}
          {answer.truncated && <p className="mt-2 text-xs text-ink-2">Mostrando as primeiras 500 linhas.</p>}
        </div>
      )}

      <details className="mt-4 text-sm">
        <summary className="cursor-pointer text-ink-2 hover:text-ink">Ver a consulta SQL</summary>
        <pre className="mt-2 overflow-x-auto rounded-lg bg-hover p-3 font-mono text-xs whitespace-pre-wrap text-ink">
          {answer.sql}
        </pre>
      </details>
      {answer.cached && <p className="mt-2 text-xs text-ink-2">Resposta guardada de uma pergunta igual feita antes.</p>}
    </figure>
  );
}
