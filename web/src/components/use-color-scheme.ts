import { useSyncExternalStore } from "react";

// Tema claro/escuro do sistema. useSyncExternalStore assina uma fonte externa
// (a media query) e re-renderiza quando ela muda. No servidor não existe tema
// (devolve null): ali os gráficos e o mapa só reservam o espaço.
const DARK_QUERY = "(prefers-color-scheme: dark)";

function subscribe(onChange: () => void) {
  const media = window.matchMedia(DARK_QUERY);
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}
const getScheme = () => (window.matchMedia(DARK_QUERY).matches ? "dark" : "light");
const getServerScheme = () => null;

export function useColorScheme(): "light" | "dark" | null {
  return useSyncExternalStore(subscribe, getScheme, getServerScheme);
}

/** Lê o valor atual de uma variável CSS do tema (ver globals.css). */
export function cssToken(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}
