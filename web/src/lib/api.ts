/**
 * Cliente da API do Radar Econômico (contrato em api/openapi.yaml).
 *
 * Só é usado no servidor do Next: o navegador nunca chama a API diretamente,
 * por isso não precisa de CORS e a URL da API não aparece para o usuário.
 */

/** Um valor da série. `date` vem como AAAA-MM-DD. */
export interface Point {
  date: string;
  value: number;
}

export interface Indicator {
  id: string;
  name: string;
  unit: string;
  frequency: "daily" | "monthly";
  source: string;
  source_code: string;
  updated_at: string;
}

export interface IndicatorWithLatest extends Indicator {
  latest: Point;
}

/** Primeira data exibida no dashboard (a coleta das séries diárias começa em 2000). */
export const SERIES_START = "2000-01-01";

// Lida a cada chamada, e não quando o módulo carrega: no Docker e no deploy a
// URL vem do ambiente em tempo de execução, não do momento do build.
function apiUrl(): string {
  return process.env.API_URL ?? "http://localhost:8080";
}

// Em produção, a API (Render Free) e o banco (Neon) desligam quando ficam sem
// uso. A API é mantida acordada (workflow keepalive.yml), mas, se o
// agendamento atrasar, ela leva ~1 min para religar: a espera cobre isso.
const TIMEOUT_MS = 70_000;

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(apiUrl() + path, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  if (!res.ok) {
    throw new Error(`a API respondeu ${res.status} em ${path}`);
  }
  return (await res.json()) as T;
}

export async function listIndicators(): Promise<IndicatorWithLatest[]> {
  const body = await getJSON<{ data: IndicatorWithLatest[] }>("/v1/indicators");
  return body.data;
}

export async function getSeries(id: string): Promise<Point[]> {
  const path = `/v1/indicators/${encodeURIComponent(id)}/values?from=${SERIES_START}`;
  const body = await getJSON<{ data: Point[] }>(path);
  return body.data;
}

// --- Combustíveis ---------------------------------------------------------------

export interface Fuel {
  id: string;
  name: string;
  unit: string;
  updated_at: string;
}

/** Média de uma semana. `week_start` é a segunda-feira (AAAA-MM-DD). */
export interface WeekPrice {
  week_start: string;
  avg_price: number;
  n_stations: number;
}

export interface FuelWithLatest extends Fuel {
  latest: WeekPrice;
}

/** Preço de um combustível num município, num mês. */
export interface MunicipalPrice {
  ibge_code: number;
  name: string;
  uf: string;
  latitude: number;
  longitude: number;
  avg_price: number;
  min_price: number;
  max_price: number;
  n_stations: number;
}

export interface FuelMunicipalities {
  month: string | null; // AAAA-MM
  available_months: string[];
  data: MunicipalPrice[];
}

export async function listFuels(): Promise<FuelWithLatest[]> {
  const body = await getJSON<{ data: FuelWithLatest[] }>("/v1/fuels");
  return body.data;
}

/** Série semanal no Brasil ("BR") ou numa UF. */
export async function getFuelPrices(id: string, scope: string): Promise<WeekPrice[]> {
  const path = `/v1/fuels/${encodeURIComponent(id)}/prices?scope=${encodeURIComponent(scope)}`;
  const body = await getJSON<{ data: WeekPrice[] }>(path);
  return body.data;
}

/** Preço por município num mês (null = o mais recente). */
export async function getFuelMunicipalities(id: string, month: string | null): Promise<FuelMunicipalities> {
  const query = month ? `?month=${encodeURIComponent(month)}` : "";
  return getJSON<FuelMunicipalities>(`/v1/fuels/${encodeURIComponent(id)}/municipalities${query}`);
}
