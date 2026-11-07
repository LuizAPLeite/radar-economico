/**
 * Lógica da página de combustíveis: funções puras, sem rede e sem DOM,
 * testadas com `node --test` (ver fuels.test.ts).
 */
import type { MunicipalPrice, WeekPrice } from "./api";
import { formatDate, formatMonth, formatNumber, formatSigned, type Card } from "./indicators.ts";

// --- UFs ------------------------------------------------------------------------

export const UF_NAMES: Record<string, string> = {
  AC: "Acre", AL: "Alagoas", AM: "Amazonas", AP: "Amapá", BA: "Bahia", CE: "Ceará",
  DF: "Distrito Federal", ES: "Espírito Santo", GO: "Goiás", MA: "Maranhão",
  MG: "Minas Gerais", MS: "Mato Grosso do Sul", MT: "Mato Grosso", PA: "Pará",
  PB: "Paraíba", PE: "Pernambuco", PI: "Piauí", PR: "Paraná", RJ: "Rio de Janeiro",
  RN: "Rio Grande do Norte", RO: "Rondônia", RR: "Roraima", RS: "Rio Grande do Sul",
  SC: "Santa Catarina", SE: "Sergipe", SP: "São Paulo", TO: "Tocantins",
};

/** Siglas em ordem alfabética do nome do estado (como as pessoas procuram). */
export const UFS = Object.keys(UF_NAMES).sort((a, b) => UF_NAMES[a].localeCompare(UF_NAMES[b], "pt-BR"));

// --- Combustíveis ---------------------------------------------------------------

/** Ordem e rótulos curtos dos botões (os nomes da API são mais longos). */
const FUEL_LABELS: Record<string, string> = {
  gasolina: "Gasolina",
  gasolina_aditivada: "Gasolina aditivada",
  etanol: "Etanol",
  diesel_s10: "Diesel S10",
  diesel: "Diesel S500",
  gnv: "GNV",
};

export function fuelLabel(fuel: { id: string; name: string }): string {
  return FUEL_LABELS[fuel.id] ?? fuel.name;
}

/** Combustíveis conhecidos na ordem acima; um novo vindo da API vai para o fim. */
export function sortFuels<T extends { id: string }>(fuels: T[]): T[] {
  const order = Object.keys(FUEL_LABELS);
  const rank = (id: string) => (order.includes(id) ? order.indexOf(id) : order.length);
  return fuels.toSorted((a, b) => rank(a.id) - rank(b.id));
}

/** "R$/l" → "por litro"; "R$/m³" → "por m³". */
export function unitPhrase(unit: string): string {
  return unit === "R$/l" ? "por litro" : `por ${unit.replace("R$/", "")}`;
}

// --- Filtros (parâmetros da URL) ------------------------------------------------

export const DEFAULT_FUEL = "gasolina";

export interface FuelFilters {
  fuel: string;
  uf: string | null; // null = Brasil
  month: string | null; // AAAA-MM; null = o mais recente
}

export type SearchParams = Record<string, string | string[] | undefined>;

function first(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

/**
 * Lê os filtros da URL (?combustivel=etanol&uf=SP&mes=2026-10). Valor
 * inválido não quebra a página: vira o padrão, como se não tivesse sido
 * informado.
 */
export function parseFuelFilters(params: SearchParams, fuelIds: string[]): FuelFilters {
  const fuel = first(params.combustivel);
  const uf = first(params.uf)?.toUpperCase();
  const month = first(params.mes);
  return {
    fuel: fuel && fuelIds.includes(fuel) ? fuel : fuelIds.includes(DEFAULT_FUEL) ? DEFAULT_FUEL : fuelIds[0],
    uf: uf && uf in UF_NAMES ? uf : null,
    month: month && /^\d{4}-(0[1-9]|1[0-2])$/.test(month) ? month : null,
  };
}

/** O caminho da página com os filtros dados (só os que não são o padrão). */
export function fuelsHref(filters: FuelFilters): string {
  const query = new URLSearchParams();
  if (filters.fuel !== DEFAULT_FUEL) query.set("combustivel", filters.fuel);
  if (filters.uf) query.set("uf", filters.uf);
  if (filters.month) query.set("mes", filters.month);
  const qs = query.toString();
  return qs ? `/combustiveis?${qs}` : "/combustiveis";
}

// --- Série semanal --------------------------------------------------------------

const DAY_MS = 24 * 60 * 60 * 1000;

function addDays(iso: string, days: number): string {
  // Date.UTC trabalha em UTC: somar dias não sofre com fuso nem horário de verão.
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d) + days * DAY_MS).toISOString().slice(0, 10);
}

/**
 * Transforma a série em pares [semana, preço] com `null` nas semanas que
 * faltam (a API omite as semanas sem pesquisa ou incompletas). Com o null,
 * o gráfico interrompe a linha em vez de ligar os pontos por cima do buraco.
 */
export function fillWeeklyGaps(points: WeekPrice[]): [string, number | null][] {
  if (points.length === 0) return [];
  const byWeek = new Map(points.map((p) => [p.week_start, p.avg_price]));
  const out: [string, number | null][] = [];
  const last = points[points.length - 1].week_start;
  for (let week = points[0].week_start; week <= last; week = addDays(week, 7)) {
    out.push([week, byWeek.get(week) ?? null]);
  }
  return out;
}

/**
 * Variação percentual da semana mais recente em relação a `weeks` semanas
 * antes. Se aquela semana não tiver dado (buraco na série), devolve null:
 * comparar com outra semana mudaria o período da comparação sem avisar.
 */
export function changeOverWeeks(points: WeekPrice[], weeks = 4): { percent: number; since: string } | null {
  if (points.length === 0) return null;
  const last = points[points.length - 1];
  const since = addDays(last.week_start, -7 * weeks);
  const before = points.find((p) => p.week_start === since);
  if (!before || before.avg_price === 0) return null;
  return { percent: ((last.avg_price - before.avg_price) / before.avg_price) * 100, since };
}

/** Diferença entre o preço da UF e o do Brasil, na semana mais recente da UF. */
export interface NationalComparison {
  week: string;
  diff: number; // positivo: a UF é mais cara
}

export function compareWithNational(state: WeekPrice[], national: WeekPrice[]): NationalComparison | null {
  const last = state.at(-1);
  if (!last) return null;
  const br = national.find((p) => p.week_start === last.week_start);
  return br ? { week: last.week_start, diff: last.avg_price - br.avg_price } : null;
}

/** "R$ 0,12 abaixo da média do Brasil na semana de 26/10/2026". */
export function comparisonText({ week, diff }: NationalComparison): string {
  const cents = toCents(Math.abs(diff));
  const when = `na semana de ${formatDate(week)}`;
  if (cents === 0) return `igual à média do Brasil ${when}`;
  return `${formatPrice(cents / 100)} ${diff > 0 ? "acima" : "abaixo"} da média do Brasil ${when}`;
}

// --- Municípios -----------------------------------------------------------------

/** Mínimo de postos para um município entrar no ranking de mais barato/caro:
 * com um posto só, o "preço da cidade" é o preço de um posto. */
export const MIN_STATIONS_FOR_RANKING = 3;

/**
 * Os `n` municípios mais baratos e os `n` mais caros. Com poucos municípios,
 * as listas encolhem para não se cruzarem (o mesmo município nas duas).
 */
export function rankMunicipalities(
  data: MunicipalPrice[],
  n: number,
): { cheapest: MunicipalPrice[]; priciest: MunicipalPrice[] } {
  const eligible = data
    .filter((m) => m.n_stations >= MIN_STATIONS_FOR_RANKING)
    .toSorted((a, b) => a.avg_price - b.avg_price || a.name.localeCompare(b.name, "pt-BR"));
  const k = Math.min(n, Math.floor(eligible.length / 2));
  return { cheapest: eligible.slice(0, k), priciest: eligible.slice(eligible.length - k).reverse() };
}

// --- Escala de cores do mapa ------------------------------------------------------

/** Quantos tons tem a escala sequencial (--seq-1 a --seq-5 em globals.css). */
export const MAP_STEPS = 5;

/** Uma faixa de preço do mapa. `from` e `to` em reais; `step` é o tom (1 a 5). */
export interface MapClass {
  from: number;
  to: number;
  step: number;
}

/**
 * Faixas de cor do mapa por quintis: cada faixa tem cerca de 1/5 dos
 * municípios. Numa escala linear, os poucos municípios muito caros (no
 * interior do Norte, por exemplo) deixariam quase todos os outros com a
 * mesma cor. Com muitos preços iguais, limites repetidos se juntam e a
 * faixa vazia some. As faixas não se sobrepõem: 6,19–6,33 e 6,34–6,50.
 */
export function mapScale(prices: number[]): MapClass[] {
  if (prices.length === 0) return [];
  const cents = prices.map(toCents).toSorted((a, b) => a - b);
  const min = cents[0];
  const max = cents[cents.length - 1];
  const starts = [min];
  for (let k = 1; k < MAP_STEPS; k++) {
    const start = cents[Math.floor((k * cents.length) / MAP_STEPS)];
    if (start > starts[starts.length - 1]) starts.push(start);
  }
  const n = starts.length;
  return starts.map((start, i) => ({
    from: start / 100,
    to: (i + 1 < n ? starts[i + 1] - 1 : max) / 100,
    // Com menos de 5 faixas, os tons se espalham pela escala: a faixa mais
    // cara fica sempre com o tom mais forte.
    step: n === 1 ? 3 : 1 + Math.round((i * (MAP_STEPS - 1)) / (n - 1)),
  }));
}

/** Índice (0, 1, …) da faixa de um preço, comparando em centavos como na tela. */
export function mapClassOf(price: number, scale: MapClass[]): number {
  const cents = toCents(price);
  let i = 0;
  while (i + 1 < scale.length && cents >= toCents(scale[i + 1].from)) i++;
  return i;
}

// --- Formatação -----------------------------------------------------------------

/**
 * Preço em centavos inteiros, arredondado como na tela. Os preços vêm com 3
 * casas, e 6.505 * 100 dá 650.4999… em ponto flutuante: o Math.round erraria
 * o meio centavo. Arredondar antes para milésimos (6505) resolve: 650,5 vira
 * 651, o mesmo R$ 6,51 que o formatPrice mostra.
 */
export function toCents(price: number): number {
  return Math.round(Math.round(price * 1000) / 10);
}

/** 6.505 → "R$ 6,51": duas casas, como no preço da bomba. */
export function formatPrice(value: number): string {
  return `R$ ${formatNumber(value, 2)}`;
}

/** Faixa da legenda: "6,19–6,33" (ou um valor só, se a faixa tiver um preço). */
export function formatRange({ from, to }: MapClass): string {
  return from === to ? formatNumber(from, 2) : `${formatNumber(from, 2)}–${formatNumber(to, 2)}`;
}

// --- Cards ----------------------------------------------------------------------

function directionOf(value: number): "up" | "down" | null {
  if (value > 0) return "up";
  if (value < 0) return "down";
  return null;
}

/** Card do preço médio mais recente no recorte (o Brasil ou uma UF). */
export function priceCard(label: string, unit: string, points: WeekPrice[]): Card {
  const last = points.at(-1);
  if (!last) {
    return {
      id: "price",
      label,
      value: "—",
      context: "sem pesquisa da ANP nos últimos 12 meses",
      detail: null,
      direction: null,
    };
  }
  const change = changeOverWeeks(points);
  // A seta segue o número exibido: +0,04% aparece como "0,0%", sem seta.
  const shown = change && Math.round(change.percent * 10) / 10;
  return {
    id: "price",
    label,
    value: formatPrice(last.avg_price),
    context: `${unitPhrase(unit)}, semana de ${formatDate(last.week_start)}`,
    detail: shown === null ? null : `${formatSigned(shown, 1)}% em 4 semanas`,
    direction: shown === null ? null : directionOf(shown),
  };
}

/** Card do município mais barato ou mais caro do mês. */
export function municipalityCard(
  kind: "cheapest" | "priciest",
  municipality: MunicipalPrice | undefined,
  month: string | null,
): Card {
  const label = kind === "cheapest" ? "Município mais barato" : "Município mais caro";
  if (!municipality || !month) {
    return {
      id: kind,
      label,
      value: "—",
      context: `poucos municípios com ${MIN_STATIONS_FOR_RANKING} ou mais postos para comparar`,
      detail: null,
      direction: null,
    };
  }
  return {
    id: kind,
    label,
    value: formatPrice(municipality.avg_price),
    context: `${municipality.name} (${municipality.uf}), ${formatMonth(month)}`,
    detail: `média de ${municipality.n_stations} postos`,
    direction: null,
  };
}
