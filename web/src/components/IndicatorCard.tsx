import type { Card } from "@/lib/indicators";

const ARROWS = { up: "▲", down: "▼" } as const;

/**
 * Card com o valor mais recente de um indicador.
 *
 * A seta usa a cor do texto, não verde/vermelho: para estes indicadores,
 * "subir" não é bom nem ruim por si só (dólar alto ajuda quem exporta e
 * atrapalha quem importa). O sinal no texto (+0,25) já diz o sentido.
 */
export function IndicatorCard({ card }: { card: Card }) {
  return (
    <article className="rounded-xl border border-hairline bg-surface p-5">
      <h2 className="text-sm font-medium text-ink-2">{card.label}</h2>
      <p className="mt-2 text-3xl font-semibold tracking-tight text-ink">{card.value}</p>
      <p className="mt-1 text-sm text-ink-2">{card.context}</p>
      {card.detail && (
        <p className="mt-3 text-sm text-ink">
          {card.direction && (
            <span aria-hidden="true" className="mr-1.5 text-xs text-ink-2">
              {ARROWS[card.direction]}
            </span>
          )}
          {card.detail}
        </p>
      )}
    </article>
  );
}
