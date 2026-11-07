/**
 * Classes dos controles de filtro, usadas nas duas páginas. Ficam num módulo
 * comum (sem "use client") para os botões e seletores terem o mesmo visual.
 */

/** Botão de filtro em forma de pílula; o selecionado fica preenchido. */
export function chipClass(selected: boolean): string {
  return (
    "rounded-full border px-3 py-1 text-sm transition-colors focus-visible:outline-2 " +
    "focus-visible:outline-offset-2 focus-visible:outline-series " +
    (selected
      ? "border-ink bg-ink font-medium text-page"
      : "border-hairline text-ink-2 hover:bg-hover hover:text-ink")
  );
}

export const SELECT_CLASS =
  "rounded-lg border border-hairline bg-surface px-2.5 py-1.5 text-sm text-ink " +
  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-series";
