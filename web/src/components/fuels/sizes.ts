/**
 * Altura do mapa (menor no celular). Fica fora dos arquivos "use client"
 * porque o esqueleto de carregamento, que roda no servidor, também usa: o
 * que um módulo "use client" exporta chega ao servidor como referência, não
 * como valor (ver chart-option.ts).
 */
export const MAP_HEIGHT_CLASS = "h-[360px] sm:h-[440px]";
