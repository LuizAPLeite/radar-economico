/**
 * Classes dos controles de filtro, usadas nas duas páginas. Ficam num módulo
 * comum (sem "use client") para os botões e seletores terem o mesmo visual.
 */

/** Botão de filtro em forma de pílula; o selecionado fica preenchido. */
export function chipClass(selected: boolean): string {
  return (
    "rounded-sm border px-3 py-1 text-sm transition-colors " +
    (selected
      ? "border-accent bg-accent font-medium text-page"
      : "border-hairline text-ink-2 hover:bg-hover hover:text-ink")
  );
}

export const SELECT_CLASS =
  "rounded-sm border border-hairline bg-surface px-2.5 py-1.5 text-sm text-ink";
