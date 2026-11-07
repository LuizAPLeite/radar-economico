import assert from "node:assert/strict";
import { describe, test } from "node:test";

import type { MunicipalPrice, WeekPrice } from "./api.ts";
import {
  changeOverWeeks,
  compareWithNational,
  comparisonText,
  fillWeeklyGaps,
  formatPrice,
  formatRange,
  fuelLabel,
  fuelsHref,
  mapClassOf,
  mapScale,
  municipalityCard,
  parseFuelFilters,
  priceCard,
  rankMunicipalities,
  sortFuels,
  toCents,
  UFS,
  unitPhrase,
} from "./fuels.ts";

const FUELS = ["diesel", "etanol", "gasolina", "gnv"];
const week = (week_start: string, avg_price: number): WeekPrice => ({ week_start, avg_price, n_stations: 100 });

function city(name: string, uf: string, avg: number, stations: number): MunicipalPrice {
  return {
    ibge_code: 1,
    name,
    uf,
    latitude: -23,
    longitude: -46,
    avg_price: avg,
    min_price: avg,
    max_price: avg,
    n_stations: stations,
  };
}

describe("filtros da URL", () => {
  test("sem parâmetros: gasolina, Brasil, mês mais recente", () => {
    assert.deepEqual(parseFuelFilters({}, FUELS), { fuel: "gasolina", uf: null, month: null });
  });

  test("valores válidos são aceitos (UF em minúsculas também)", () => {
    assert.deepEqual(parseFuelFilters({ combustivel: "etanol", uf: "sp", mes: "2026-10" }, FUELS), {
      fuel: "etanol",
      uf: "SP",
      month: "2026-10",
    });
  });

  test("valores inválidos viram o padrão, sem quebrar a página", () => {
    assert.deepEqual(parseFuelFilters({ combustivel: "querosene", uf: "XX", mes: "2026-13" }, FUELS), {
      fuel: "gasolina",
      uf: null,
      month: null,
    });
  });

  test("parâmetro repetido usa o primeiro", () => {
    assert.equal(parseFuelFilters({ uf: ["RJ", "SP"] }, FUELS).uf, "RJ");
  });

  test("fuelsHref omite os padrões", () => {
    assert.equal(fuelsHref({ fuel: "gasolina", uf: null, month: null }), "/combustiveis");
    assert.equal(fuelsHref({ fuel: "etanol", uf: "SP", month: "2026-10" }), "/combustiveis?combustivel=etanol&uf=SP&mes=2026-10");
  });

  test("as 27 UFs, em ordem do nome do estado", () => {
    assert.equal(UFS.length, 27);
    assert.deepEqual(UFS.slice(0, 3), ["AC", "AL", "AP"]); // Acre, Alagoas, Amapá
  });
});

describe("série semanal", () => {
  test("fillWeeklyGaps põe null nas semanas que faltam", () => {
    const filled = fillWeeklyGaps([week("2026-03-30", 6.79), week("2026-05-04", 6.7)]);
    assert.deepEqual(filled, [
      ["2026-03-30", 6.79],
      ["2026-04-06", null],
      ["2026-04-13", null],
      ["2026-04-20", null],
      ["2026-04-27", null],
      ["2026-05-04", 6.7],
    ]);
  });

  test("fillWeeklyGaps atravessa a virada do ano", () => {
    const filled = fillWeeklyGaps([week("2025-12-29", 6.4), week("2026-01-12", 6.5)]);
    assert.deepEqual(filled.map(([w]) => w), ["2025-12-29", "2026-01-05", "2026-01-12"]);
  });

  test("changeOverWeeks compara com 4 semanas antes", () => {
    const r = changeOverWeeks([week("2026-09-28", 6.25), week("2026-10-19", 6.5), week("2026-10-26", 6.5)]);
    assert.ok(r);
    assert.equal(r.since, "2026-09-28");
    assert.ok(Math.abs(r.percent - 4) < 1e-9, `percent = ${r.percent}, quer 4`); // 6,25 → 6,50
  });

  test("changeOverWeeks devolve null se a semana de comparação for um buraco", () => {
    assert.equal(changeOverWeeks([week("2026-10-19", 6.5), week("2026-10-26", 6.5)]), null);
  });

  test("comparação com o Brasil usa a mesma semana", () => {
    const national = [week("2026-10-19", 6.4), week("2026-10-26", 6.505)];
    const state = [week("2026-10-19", 6.3), week("2026-10-26", 6.39)];
    const cmp = compareWithNational(state, national);
    assert.ok(cmp);
    assert.equal(comparisonText(cmp), "R$ 0,12 abaixo da média do Brasil na semana de 26/10/2026");
    assert.equal(
      comparisonText({ week: "2026-10-26", diff: 0.004 }),
      "igual à média do Brasil na semana de 26/10/2026",
    );
  });

  test("sem a mesma semana no Brasil, não compara", () => {
    assert.equal(compareWithNational([week("2026-10-26", 6.4)], [week("2026-10-19", 6.4)]), null);
    assert.equal(compareWithNational([], [week("2026-10-19", 6.4)]), null);
  });
});

describe("combustíveis", () => {
  test("sortFuels usa a ordem dos botões e manda os desconhecidos para o fim", () => {
    const ids = sortFuels([{ id: "gnv" }, { id: "hidrogenio" }, { id: "etanol" }, { id: "gasolina" }]).map((f) => f.id);
    assert.deepEqual(ids, ["gasolina", "etanol", "gnv", "hidrogenio"]);
  });

  test("fuelLabel e unitPhrase", () => {
    assert.equal(fuelLabel({ id: "diesel", name: "Diesel comum (S500)" }), "Diesel S500");
    assert.equal(fuelLabel({ id: "hidrogenio", name: "Hidrogênio" }), "Hidrogênio");
    assert.equal(unitPhrase("R$/l"), "por litro");
    assert.equal(unitPhrase("R$/m³"), "por m³");
  });
});

describe("cards", () => {
  test("preço médio com a variação em 4 semanas", () => {
    const card = priceCard("Gasolina · Brasil", "R$/l", [week("2026-09-28", 6.25), week("2026-10-26", 6.5)]);
    assert.equal(card.value, "R$ 6,50");
    assert.equal(card.context, "por litro, semana de 26/10/2026");
    assert.equal(card.detail, "+4,0% em 4 semanas");
    assert.equal(card.direction, "up");
  });

  test("variação que arredonda para zero fica sem seta", () => {
    const card = priceCard("GNV · Brasil", "R$/m³", [week("2026-09-28", 5), week("2026-10-26", 5.002)]);
    assert.equal(card.detail, "0,0% em 4 semanas");
    assert.equal(card.direction, null);
  });

  test("UF sem pesquisa no período", () => {
    const card = priceCard("GNV · Roraima", "R$/m³", []);
    assert.equal(card.value, "—");
    assert.equal(card.detail, null);
  });

  test("município do mês", () => {
    const card = municipalityCard("cheapest", city("Goiatuba", "GO", 5.74, 5), "2026-10");
    assert.equal(card.value, "R$ 5,74");
    assert.equal(card.context, "Goiatuba (GO), out/2026");
    assert.equal(card.detail, "média de 5 postos");
    assert.equal(municipalityCard("priciest", undefined, "2026-10").value, "—");
  });
});

describe("municípios", () => {
  const data = [
    city("Parintins", "AM", 8.99, 7),
    city("Goiatuba", "GO", 5.74, 5),
    city("Cidade de um posto só", "PI", 4.99, 1),
    city("Guarapuava", "PR", 5.86, 18),
    city("Caxias", "MA", 6.1, 4),
  ];

  test("rankMunicipalities ignora cidades com menos de 3 postos", () => {
    const { cheapest, priciest } = rankMunicipalities(data, 2);
    assert.deepEqual(cheapest.map((m) => m.name), ["Goiatuba", "Guarapuava"]);
    assert.deepEqual(priciest.map((m) => m.name), ["Parintins", "Caxias"]);
  });

  test("com poucos municípios, as listas não se cruzam", () => {
    const three = data.filter((m) => m.name !== "Caxias"); // 3 elegíveis
    const { cheapest, priciest } = rankMunicipalities(three, 5);
    assert.deepEqual(cheapest.map((m) => m.name), ["Goiatuba"]);
    assert.deepEqual(priciest.map((m) => m.name), ["Parintins"]);
    assert.deepEqual(rankMunicipalities([city("Única", "RR", 7, 9)], 5), { cheapest: [], priciest: [] });
  });

  test("formatPrice usa duas casas", () => {
    assert.equal(formatPrice(6.505), "R$ 6,51");
  });

  test("toCents arredonda o meio centavo como a tela", () => {
    assert.equal(toCents(6.505), 651);
    assert.equal(toCents(6.194), 619);
    assert.equal(toCents(0.115), 12);
  });
});

describe("escala do mapa", () => {
  test("quintis: cinco faixas que não se sobrepõem", () => {
    const prices = [5.0, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 5.9];
    const scale = mapScale(prices);
    assert.deepEqual(scale.map(formatRange), ["5,00–5,19", "5,20–5,39", "5,40–5,59", "5,60–5,79", "5,80–5,90"]);
    assert.deepEqual(scale.map((c) => c.step), [1, 2, 3, 4, 5]);
  });

  test("uns poucos preços altos não achatam a escala", () => {
    // Uma escala linear de 5,00 a 9,00 poria os oito primeiros na faixa mais clara.
    const scale = mapScale([5.0, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 8.9, 9.0]);
    assert.equal(new Set([5.0, 5.2, 5.4, 5.6, 8.9].map((p) => mapClassOf(p, scale))).size, 5);
  });

  test("preços repetidos juntam as faixas; a mais cara fica com o tom mais forte", () => {
    const scale = mapScale([6, 6, 6, 6, 7]);
    assert.deepEqual(scale.map(formatRange), ["6,00–6,99", "7,00"]);
    assert.deepEqual(scale.map((c) => c.step), [1, 5]);
    assert.deepEqual(mapScale([6.5]).map((c) => c.step), [3]);
    assert.deepEqual(mapScale([]), []);
  });

  test("a faixa de um preço segue o arredondamento da tela", () => {
    const scale = mapScale([6.0, 6.2, 6.4, 6.51, 6.8]);
    // 6,505 aparece como R$ 6,51 no balão: tem de cair na faixa que começa em 6,51.
    const i = mapClassOf(6.505, scale);
    assert.equal(scale[i].from, 6.51);
  });
});
