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

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(apiUrl() + path, { signal: AbortSignal.timeout(5_000) });
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
