// Gera public/geo/ufs.json: o contorno dos 27 estados, usado como fundo do
// mapa de combustíveis (o mapa não depende de nenhum serviço de mapas).
// Fonte: IBGE, API de malhas, qualidade "mínima" (suficiente para o país todo).
//
// Rodar da pasta web/ quando a malha do IBGE mudar:
//   node scripts/build-ufs-geojson.mjs
import { mkdir, writeFile } from "node:fs/promises";

const SOURCE =
  "https://servicodados.ibge.gov.br/api/v3/malhas/paises/BR" +
  "?intrarregiao=UF&formato=application/vnd.geo%2Bjson&qualidade=minima";

// O IBGE identifica as UFs pelo código numérico (codarea); o mapa usa a sigla.
const UF_BY_CODE = {
  11: "RO", 12: "AC", 13: "AM", 14: "RR", 15: "PA", 16: "AP", 17: "TO",
  21: "MA", 22: "PI", 23: "CE", 24: "RN", 25: "PB", 26: "PE", 27: "AL", 28: "SE", 29: "BA",
  31: "MG", 32: "ES", 33: "RJ", 35: "SP",
  41: "PR", 42: "SC", 43: "RS",
  50: "MS", 51: "MT", 52: "GO", 53: "DF",
};

// 3 casas decimais ≈ 100 m: invisível no zoom do país inteiro, e o arquivo
// fica bem menor.
const round = (n) => Math.round(n * 1000) / 1000;
const roundCoordinates = (c) => (typeof c[0] === "number" ? c.map(round) : c.map(roundCoordinates));

const res = await fetch(SOURCE);
if (!res.ok) {
  throw new Error(`IBGE respondeu ${res.status}`);
}
const geo = await res.json();

// Retângulo [oeste, sul, leste, norte] de cada UF, no membro "bbox" do GeoJSON
// (RFC 7946): o mapa usa para enquadrar o estado escolhido no filtro.
function bboxOf(coordinates) {
  const box = [Infinity, Infinity, -Infinity, -Infinity];
  const visit = (c) => {
    if (typeof c[0] === "number") {
      box[0] = Math.min(box[0], c[0]);
      box[1] = Math.min(box[1], c[1]);
      box[2] = Math.max(box[2], c[0]);
      box[3] = Math.max(box[3], c[1]);
    } else {
      c.forEach(visit);
    }
  };
  visit(coordinates);
  return box;
}

const features = geo.features.map((f) => {
  const uf = UF_BY_CODE[f.properties.codarea];
  if (!uf) {
    throw new Error(`código de UF desconhecido: ${f.properties.codarea}`);
  }
  const coordinates = roundCoordinates(f.geometry.coordinates);
  return {
    type: "Feature",
    bbox: bboxOf(coordinates),
    properties: { uf },
    geometry: { type: f.geometry.type, coordinates },
  };
});
if (features.length !== 27) {
  throw new Error(`esperava 27 UFs, vieram ${features.length}`);
}

const json = JSON.stringify({ type: "FeatureCollection", features });
await mkdir("public/geo", { recursive: true });
await writeFile("public/geo/ufs.json", json);
console.log(`public/geo/ufs.json: ${features.length} UFs, ${(json.length / 1024).toFixed(0)} KB`);
