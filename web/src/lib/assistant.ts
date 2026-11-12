/**
 * Lógica da página do assistente: funções puras, sem rede e sem DOM,
 * testadas com `node --test` (ver assistant.test.ts).
 */
import type { Answer, AnswerColumn } from "./api";
import { formatDate, formatMonth } from "./indicators.ts";

/** Mesmos limites da API (assistant.MinQuestionLength / MaxQuestionLength). */
export const MIN_QUESTION = 3;
export const MAX_QUESTION = 300;

/** Perguntas de exemplo: mostram o que dá para perguntar. */
export const EXAMPLES = [
  "Como o preço médio da gasolina variou por ano desde 2015?",
  "Qual estado tem a gasolina mais cara hoje?",
  "Qual foi o IPCA acumulado nos últimos 12 meses?",
  "Como o dólar e o petróleo Brent se moveram em 2026?",
  "Quanto a gasolina acompanha o petróleo em cada período da política de preços?",
] as const;

/** Mensagem de erro para a pergunta digitada, ou null se ela serve. */
export function validateQuestion(question: string): string | null {
  const length = [...question.trim()].length; // conta caracteres, não bytes
  if (length < MIN_QUESTION) return "Escreva uma pergunta.";
  if (length > MAX_QUESTION) return `A pergunta pode ter até ${MAX_QUESTION} caracteres.`;
  return null;
}

/** Mensagem para cada código de erro da API (POST /v1/ask). */
export function errorMessage(code: string): string {
  switch (code) {
    case "invalid_question":
    case "invalid_body":
      return `A pergunta precisa ter de ${MIN_QUESTION} a ${MAX_QUESTION} caracteres.`;
    case "assistant_limit":
      return "O assistente atingiu o limite de perguntas por agora (ele usa o plano grátis de um modelo de linguagem). Tente daqui a pouco, ou clique numa pergunta de exemplo: as respostas guardadas continuam disponíveis.";
    case "unanswerable":
      return "Não consegui montar uma consulta para essa pergunta. Tente reformular, citando o indicador ou o combustível e o período.";
    case "assistant_disabled":
      return "O assistente não está ligado neste servidor.";
    default:
      return "O assistente está indisponível no momento. Tente de novo em instantes.";
  }
}

const number = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 3 });

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

/** De quanto em quanto tempo é uma coluna de datas: por ano, por mês ou por dia. */
export type DateGranularity = "year" | "month" | "day";

/**
 * Todas as datas no dia 1º de janeiro → ano; todas no dia 1º → mês; senão,
 * dia. Assim um date_trunc('year', ...) aparece como "2015", e não "01/01/2015".
 */
export function dateGranularity(values: unknown[]): DateGranularity {
  const dates = values.filter((v): v is string => typeof v === "string" && ISO_DATE.test(v));
  if (dates.length === 0) return "day";
  if (dates.every((d) => d.endsWith("-01-01"))) return "year";
  if (dates.every((d) => d.endsWith("-01"))) return "month";
  return "day";
}

/** "2026-09-01" → "2026", "set/2026" ou "01/09/2026", conforme a granularidade. */
export function formatDateAs(iso: string, granularity: DateGranularity): string {
  if (!ISO_DATE.test(iso)) return iso;
  if (granularity === "year") return iso.slice(0, 4);
  if (granularity === "month") return formatMonth(iso);
  return formatDate(iso);
}

/** Valor de uma célula para a tabela: datas conforme a granularidade, números em pt-BR. */
export function formatCell(
  value: string | number | boolean | null,
  type: AnswerColumn["type"],
  granularity: DateGranularity = "day",
): string {
  if (value === null) return "—";
  if (type === "number" && typeof value === "number") return number.format(value);
  if (type === "date" && typeof value === "string") return formatDateAs(value, granularity);
  if (typeof value === "boolean") return value ? "sim" : "não";
  return String(value);
}

/** "preco_medio" → "Preço médio"? Sem acentos não dá para adivinhar: só troca _ e capitaliza. */
export function columnLabel(name: string): string {
  const text = name.replaceAll("_", " ").trim();
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** No máximo duas séries no gráfico: a principal (cor de destaque) e uma de referência (cinza). */
export const MAX_CHART_SERIES = 2;

export interface ChartData {
  kind: "line" | "bar";
  xType: AnswerColumn["type"];
  granularity: DateGranularity; // das datas do eixo X
  x: string[];
  series: { name: string; values: (number | null)[] }[];
  /** Séries que ficaram fora do gráfico por estarem noutra escala (só na tabela). */
  omitted: string[];
}

/** Escalas mais distantes que isto não dividem um eixo: a menor vira uma linha achatada. */
export const MAX_SCALE_RATIO = 5;

function typicalSize(values: (number | null)[]): number {
  const sorted = values.filter((v): v is number => v !== null).map(Math.abs).toSorted((a, b) => a - b);
  return sorted.length === 0 ? 0 : sorted[Math.floor(sorted.length / 2)];
}

/**
 * Os dados do gráfico sugerido pela API, já separados por coluna. A API só
 * manda line/bar quando as colunas existem e os tipos batem; aqui ainda se
 * confere, para uma resposta inesperada virar "sem gráfico" (a tabela
 * continua lá) em vez de quebrar a página.
 */
export function chartData(answer: Answer): ChartData | null {
  const { chart, columns, rows } = answer;
  if (chart.type === "none" || rows.length === 0) return null;
  const xIndex = columns.findIndex((c) => c.name === chart.x);
  if (xIndex < 0) return null;
  const candidates = chart.y.flatMap((name) => {
    const i = columns.findIndex((c) => c.name === name && c.type === "number");
    if (i < 0) return [];
    return [{ name: columnLabel(name), values: rows.map((r) => (typeof r[i] === "number" ? r[i] : null)) }];
  });
  if (candidates.length === 0) return null;

  // Um eixo só (nunca dois eixos Y): série noutra escala, ou além do limite
  // de cores, fica só na tabela.
  const base = typicalSize(candidates[0].values);
  const fits = (s: (typeof candidates)[number]) => {
    const size = typicalSize(s.values);
    if (base === 0 || size === 0) return size === base;
    return Math.max(base, size) / Math.min(base, size) <= MAX_SCALE_RATIO;
  };
  const series = [candidates[0], ...candidates.slice(1).filter(fits)].slice(0, MAX_CHART_SERIES);
  const omitted = candidates.filter((c) => !series.includes(c)).map((c) => c.name);

  const x = rows.map((r) => (r[xIndex] === null ? "" : String(r[xIndex])));
  return {
    kind: chart.type,
    xType: columns[xIndex].type,
    granularity: dateGranularity(x),
    x,
    series,
    omitted,
  };
}
