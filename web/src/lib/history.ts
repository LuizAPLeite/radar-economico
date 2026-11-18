/**
 * Série oficial da ANP para a linha do tempo da página inicial: as semanas
 * viram médias mensais (cerca de 270 pontos de 2004 até hoje, em vez de
 * ~1.150), o bastante para o traço e leve para enviar ao navegador.
 */

export interface MonthPrice {
  month: string; // AAAA-MM
  price: number;
}

export interface Milestone {
  month: string; // AAAA-MM
  text: string;
}

/** Mudanças na política de preços da Petrobras (as mesmas de pipeline/.../regimes.py). */
export const MILESTONES: Milestone[] = [
  { month: "2016-10", text: "A Petrobras passa a seguir o preço internacional do petróleo." },
  { month: "2023-05", text: "A Petrobras deixa de seguir o preço internacional à risca." },
];

/** Média de cada mês, em ordem. Semanas sem preço não entram. */
export function monthlyAverages(weeks: { week_start: string; avg_price: number }[]): MonthPrice[] {
  const sums = new Map<string, { total: number; n: number }>();
  for (const w of weeks) {
    const month = w.week_start.slice(0, 7);
    const acc = sums.get(month) ?? { total: 0, n: 0 };
    acc.total += w.avg_price;
    acc.n += 1;
    sums.set(month, acc);
  }
  return [...sums.entries()]
    .toSorted(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([month, { total, n }]) => ({ month, price: total / n }));
}

const MONTHS = ["jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"];

/** "2016-10" → "out/2016". */
export function formatMonthShort(month: string): string {
  const [year, m] = month.split("-");
  return `${MONTHS[Number(m) - 1]}/${year}`;
}

const brl = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });

/** 6.512 → "R$ 6,51" (com espaço comum, não o não-separável do Intl). */
export function formatBRL(value: number): string {
  return brl.format(value).replace(/ /g, " ");
}

/** Índice do ponto mais próximo de uma fração (0 a 1) do caminho. */
export function indexAt(fraction: number, length: number): number {
  if (length === 0) return 0;
  const f = Math.min(1, Math.max(0, fraction));
  return Math.round(f * (length - 1));
}

/** Média de cada ano, para a tabela acessível. */
export function yearlyAverages(months: MonthPrice[]): { year: string; price: number }[] {
  const sums = new Map<string, { total: number; n: number }>();
  for (const { month, price } of months) {
    const year = month.slice(0, 4);
    const acc = sums.get(year) ?? { total: 0, n: 0 };
    acc.total += price;
    acc.n += 1;
    sums.set(year, acc);
  }
  return [...sums.entries()].map(([year, { total, n }]) => ({ year, price: total / n }));
}
