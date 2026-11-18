"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type KeyboardEvent,
  type PointerEvent,
} from "react";

import { formatBRL, formatMonthShort, indexAt, MILESTONES, type MonthPrice } from "@/lib/history";

// Área do desenho em unidades do viewBox (o SVG escala com a largura).
const W = 1000;
const H = 360;
const PAD = { top: 12, right: 8, bottom: 8, left: 8 };

/** Anos marcados no eixo: o início, os marcos da política de preços e o último. */
function axisYears(months: MonthPrice[]): string[] {
  const first = months[0].month.slice(0, 4);
  const last = months.at(-1)!.month.slice(0, 4);
  return [...new Set([first, "2010", "2016", "2023", last])].filter((y) => y >= first && y <= last);
}

/** Linhas finas de cédula: senoides sobrepostas, desenhadas uma vez (decorativas). */
function guillochePaths(): string[] {
  const paths: string[] = [];
  for (let k = 0; k < 9; k++) {
    let d = "";
    for (let x = 0; x <= W; x += 8) {
      const y = H / 2 + Math.sin(x / (38 + k * 3) + k) * (60 + k * 9) + Math.sin(x / 140 + k * 0.7) * 30;
      d += `${x === 0 ? "M" : "L"}${x},${y.toFixed(1)}`;
    }
    paths.push(d);
  }
  return paths;
}

const REDUCED = "(prefers-reduced-motion: reduce)";

/** Preferência "reduzir movimento" do sistema, acompanhando mudanças. */
function useReducedMotion(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const q = window.matchMedia(REDUCED);
      q.addEventListener("change", onChange);
      return () => q.removeEventListener("change", onChange);
    },
    () => window.matchMedia(REDUCED).matches,
    () => false, // no servidor
  );
}

/**
 * O preço da gasolina de 2004 até hoje, contado pela rolagem: enquanto a
 * seção passa, a linha se desenha e o número grande acompanha o mês. Passar
 * o mouse (ou usar as setas do teclado) mostra qualquer mês. Com "reduzir
 * movimento" ligado no sistema, a linha já aparece inteira e parada.
 */
export function PriceTimeline({ months }: { months: MonthPrice[] }) {
  const section = useRef<HTMLElement>(null);
  const reduced = useReducedMotion();
  const [progress, setProgress] = useState(0); // o HTML sai no começo da série, como no topo da página
  const [inspected, setInspected] = useState<number | null>(null);

  // Rolagem → progresso de 0 a 1 enquanto a seção alta atravessa a tela.
  useEffect(() => {
    if (reduced) return;
    let frame = 0;
    const update = () => {
      frame = 0;
      const el = section.current;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      const travel = rect.height - window.innerHeight;
      setProgress(travel <= 0 ? 1 : Math.min(1, Math.max(0, -rect.top / travel)));
    };
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    frame = requestAnimationFrame(update);
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      cancelAnimationFrame(frame);
    };
  }, [reduced]);

  const { path, points, min, max } = useMemo(() => {
    const prices = months.map((m) => m.price);
    const min = Math.min(...prices);
    const max = Math.max(...prices);
    const x = (i: number) => PAD.left + (i / (months.length - 1)) * (W - PAD.left - PAD.right);
    const y = (v: number) => PAD.top + (1 - (v - min) / (max - min)) * (H - PAD.top - PAD.bottom);
    const points = months.map((m, i) => ({ x: x(i), y: y(m.price) }));
    const path = toPath(points);
    return { path, points, min, max };
  }, [months]);
  const guilloche = useMemo(() => guillochePaths(), []);

  const scrolled = indexAt(reduced ? 1 : progress, months.length);
  const current = inspected ?? scrolled;
  const point = points[current];
  // O trecho percorrido vai até o mês da rolagem (medido no tempo, não no
  // comprimento do traço, que é maior onde a linha oscila mais).
  const travelled = useMemo(() => toPath(points.slice(0, scrolled + 1)), [points, scrolled]);
  const month = months[current];
  const note = MILESTONES.find((m) => Math.abs(monthDistance(m.month, month.month)) <= 8);

  const inspectAt = useCallback(
    (e: PointerEvent<SVGSVGElement>) => {
      const rect = e.currentTarget.getBoundingClientRect();
      const fraction = (e.clientX - rect.left) / rect.width;
      setInspected(indexAt(fraction, months.length));
    },
    [months.length],
  );

  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const step = e.shiftKey ? 12 : 1;
    const from = inspected ?? scrolled;
    const next =
      e.key === "ArrowRight" ? from + step
      : e.key === "ArrowLeft" ? from - step
      : e.key === "Home" ? 0
      : e.key === "End" ? months.length - 1
      : null;
    if (next === null) return;
    e.preventDefault();
    setInspected(Math.min(months.length - 1, Math.max(0, next)));
  };

  const first = months[0];
  const last = months.at(-1)!;
  const years = axisYears(months);

  return (
    <section
      ref={section}
      aria-labelledby="timeline-title"
      // Alta para a rolagem ter o que percorrer; com movimento reduzido, só a tela.
      className={reduced ? "relative" : "relative h-[260vh]"}
    >
      <div className={reduced ? "" : "sticky top-0 flex h-svh flex-col justify-center"}>
        <div className="mx-auto w-full max-w-5xl px-4 py-10">
          <h1 id="timeline-title" className="max-w-xl text-lg text-ink-2 sm:text-xl">
            O litro da gasolina no Brasil, pela média oficial da ANP
          </h1>
          <p className="mt-3 flex flex-wrap items-baseline gap-x-5 gap-y-1" aria-live="off">
            <span className="figure text-[clamp(4.5rem,17vw,11.5rem)] font-semibold text-ink">
              {formatBRL(month.price)}
            </span>
            <span className="figure text-[clamp(2rem,6vw,3.75rem)] font-medium text-accent">
              {formatMonthShort(month.month)}
            </span>
          </p>
          <p className="mt-4 min-h-[3rem] max-w-md text-base text-ink-2">
            {note ? (
              <>
                <span className="font-medium text-ink">{formatMonthShort(note.month)}.</span> {note.text}
              </>
            ) : current === months.length - 1 ? (
              <>
                Era {formatBRL(first.price)} em {formatMonthShort(first.month)}. Passe o mouse ou toque na linha
                para ver qualquer mês.
              </>
            ) : (
              <>Role a página para atravessar {Number(last.month.slice(0, 4)) - Number(first.month.slice(0, 4))} anos.</>
            )}
          </p>

          <div
            role="slider"
            tabIndex={0}
            aria-label="Mês da linha do tempo"
            aria-valuemin={0}
            aria-valuemax={months.length - 1}
            aria-valuenow={current}
            aria-valuetext={`${formatMonthShort(month.month)}: ${formatBRL(month.price)}`}
            onKeyDown={onKey}
            onBlur={() => setInspected(null)}
            className="mt-6 rounded-sm"
          >
            {/* O SVG estica para caber na altura da tela (preserveAspectRatio
                none): os traços não engrossam (non-scaling-stroke), e o que não
                pode deformar (o ponto e os anos) é HTML posicionado por cima. */}
            <div className="relative">
              <svg
                viewBox={`0 0 ${W} ${H}`}
                preserveAspectRatio="none"
                className="block h-[34svh] min-h-44 w-full touch-pan-y select-none"
                onPointerMove={inspectAt}
                onPointerDown={inspectAt}
                onPointerLeave={() => setInspected(null)}
                aria-hidden="true"
              >
                <g fill="none" stroke="var(--guilloche)" strokeWidth="0.5" opacity="0.3">
                  {guilloche.map((d, k) => (
                    <path key={k} d={d} vectorEffect="non-scaling-stroke" />
                  ))}
                </g>
                {MILESTONES.map((m) => {
                  const i = months.findIndex((x) => x.month === m.month);
                  if (i < 0) return null;
                  return (
                    <line
                      key={m.month}
                      x1={points[i].x}
                      x2={points[i].x}
                      y1={PAD.top}
                      y2={H - PAD.bottom}
                      stroke="var(--axis)"
                      strokeDasharray="3 4"
                      vectorEffect="non-scaling-stroke"
                    />
                  );
                })}
                {/* O traço inteiro, apagado, e por cima o trecho já percorrido. */}
                <path d={path} fill="none" stroke="var(--axis)" strokeWidth="1.25" vectorEffect="non-scaling-stroke" />
                <path
                  d={travelled}
                  fill="none"
                  stroke="var(--series-1)"
                  strokeWidth="3"
                  strokeLinejoin="round"
                  vectorEffect="non-scaling-stroke"
                />
                <line
                  x1={point.x}
                  x2={point.x}
                  y1={PAD.top}
                  y2={H - PAD.bottom}
                  stroke="var(--ink)"
                  strokeOpacity="0.35"
                  vectorEffect="non-scaling-stroke"
                />
              </svg>
              <span
                aria-hidden="true"
                className="pointer-events-none absolute size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-[3px] border-series bg-page"
                style={{ left: `${(point.x / W) * 100}%`, top: `${(point.y / H) * 100}%` }}
              />
              <div aria-hidden="true" className="relative h-5 text-sm text-muted">
                {years.map((y) => {
                  const i = months.findIndex((m) => m.month.startsWith(y));
                  const edge = i === 0 ? "translate-x-0" : i > months.length - 20 ? "-translate-x-full" : "-translate-x-1/2";
                  // No celular só cabem o início, 2016 e o fim.
                  const narrow = y === "2010" || y === "2023" ? "hidden sm:inline" : "";
                  return (
                    <span key={y} className={`absolute top-0 ${edge} ${narrow}`} style={{ left: `${(points[i].x / W) * 100}%` }}>
                      {y}
                    </span>
                  );
                })}
              </div>
            </div>
          </div>
          <p className="mt-2 text-sm text-muted">
            Mínimo de {formatBRL(min)} e máximo de {formatBRL(max)} no período. Fonte: ANP, Levantamento de Preços de
            Combustíveis.
          </p>
        </div>
      </div>
    </section>
  );
}

/** Caminho SVG que liga os pontos em ordem. */
function toPath(points: { x: number; y: number }[]): string {
  return points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join("");
}

/** Meses entre dois AAAA-MM (b − a). */
function monthDistance(a: string, b: string): number {
  const [ya, ma] = a.split("-").map(Number);
  const [yb, mb] = b.split("-").map(Number);
  return (yb - ya) * 12 + (mb - ma);
}
