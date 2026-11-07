// Copia o worker do MapLibre para public/vendor/.
//
// O MapLibre processa os dados do mapa num Web Worker, um script separado
// que o navegador baixa por URL. A partir da versão 6, quem usa um bundler
// (o Turbopack do Next) precisa servir esse arquivo e informar o endereço com
// setWorkerUrl (ver src/components/fuels/FuelMap.tsx): o bundler não o inclui
// sozinho. O arquivo copiado não vai para o git (está no .gitignore).
//
// Roda sozinho antes do `npm run dev` e do `npm run build` (predev/prebuild).
import { copyFile, mkdir } from "node:fs/promises";

const source = new URL(import.meta.resolve("maplibre-gl/dist/maplibre-gl-worker.mjs"));
const target = new URL("../public/vendor/maplibre-gl-worker.mjs", import.meta.url);

await mkdir(new URL(".", target), { recursive: true });
await copyFile(source, target);
console.log("public/vendor/maplibre-gl-worker.mjs copiado");
