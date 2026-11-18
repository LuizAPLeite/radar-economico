import assert from "node:assert/strict";
import { describe, test } from "node:test";

import { formatBRL, formatMonthShort, indexAt, monthlyAverages, yearlyAverages } from "./history.ts";

describe("monthlyAverages", () => {
  test("agrupa as semanas por mês, em ordem", () => {
    const months = monthlyAverages([
      { week_start: "2004-06-07", avg_price: 2 },
      { week_start: "2004-05-10", avg_price: 1 },
      { week_start: "2004-05-17", avg_price: 3 },
    ]);
    assert.deepEqual(months, [
      { month: "2004-05", price: 2 },
      { month: "2004-06", price: 2 },
    ]);
  });
  test("série vazia", () => {
    assert.deepEqual(monthlyAverages([]), []);
  });
});

describe("indexAt", () => {
  test("mapeia a fração do caminho para o ponto mais próximo", () => {
    assert.equal(indexAt(0, 10), 0);
    assert.equal(indexAt(1, 10), 9);
    assert.equal(indexAt(0.5, 11), 5);
  });
  test("fora de 0 a 1 fica nas pontas", () => {
    assert.equal(indexAt(-1, 10), 0);
    assert.equal(indexAt(2, 10), 9);
  });
});

test("formatos", () => {
  assert.equal(formatMonthShort("2016-10"), "out/2016");
  assert.equal(formatBRL(6.512), "R$ 6,51");
});

test("yearlyAverages", () => {
  assert.deepEqual(
    yearlyAverages([
      { month: "2004-05", price: 2 },
      { month: "2004-06", price: 4 },
      { month: "2005-01", price: 5 },
    ]),
    [
      { year: "2004", price: 3 },
      { year: "2005", price: 5 },
    ],
  );
});
