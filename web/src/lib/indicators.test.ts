// Testes com o executor nativo do Node (node --test): o Node 26 roda
// TypeScript direto, sem compilar e sem bibliotecas extras de teste.
import assert from "node:assert/strict";
import { describe, test } from "node:test";

import type { IndicatorWithLatest, Point } from "./api.ts";
import {
  accumulated12Months,
  buildCard,
  formatDate,
  formatIndicatorValue,
  formatMonth,
  formatNumber,
  formatSigned,
  formatUpdatedAt,
  lastChangePercent,
  lastLevelChange,
  periodStart,
} from "./indicators.ts";

const p = (date: string, value: number): Point => ({ date, value });

/** 12 meses seguidos (2025-10 a 2026-09) com o mesmo valor. */
function months(value: number): Point[] {
  return Array.from({ length: 12 }, (_, i) => {
    const month = ((9 + i) % 12) + 1;
    const year = 2025 + Math.floor((9 + i) / 12);
    return p(`${year}-${String(month).padStart(2, "0")}-01`, value);
  });
}

function indicator(id: string, name: string, latest: Point): IndicatorWithLatest {
  return {
    id,
    name,
    unit: "",
    frequency: "daily",
    source: "BCB",
    source_code: "0",
    updated_at: "2026-11-07T18:39:18Z",
    latest,
  };
}

describe("formatação", () => {
  test("formatDate não sofre com fuso horário", () => {
    assert.equal(formatDate("2026-11-06"), "06/11/2026");
    assert.equal(formatDate("2000-01-01"), "01/01/2000");
  });

  test("formatMonth", () => {
    assert.equal(formatMonth("2026-09-01"), "set/2026");
    assert.equal(formatMonth("2026-01-01"), "jan/2026");
  });

  test("números no padrão brasileiro", () => {
    assert.equal(formatNumber(5.5013, 4), "5,5013");
    assert.equal(formatNumber(1234.5, 2), "1.234,50");
    assert.equal(formatSigned(0.25, 2), "+0,25");
    assert.equal(formatSigned(-0.32, 2), "-0,32");
    assert.equal(formatSigned(0, 2), "0,00");
  });

  test("formatUpdatedAt usa o horário de Brasília", () => {
    assert.equal(formatUpdatedAt("2026-11-07T18:39:18.085614Z"), "07/11/2026 às 15:39");
    // 01:30 em UTC ainda é o dia anterior em Brasília
    assert.equal(formatUpdatedAt("2026-11-08T01:30:00Z"), "07/11/2026 às 22:30");
  });

  test("formatIndicatorValue", () => {
    assert.equal(formatIndicatorValue("usd_brl", 5.5013), "R$ 5,5013");
    assert.equal(formatIndicatorValue("selic", 13.75), "13,75%");
    assert.equal(formatIndicatorValue("ipca", -0.32), "-0,32%");
  });
});

describe("cálculos", () => {
  test("lastChangePercent", () => {
    const r = lastChangePercent([p("2026-11-05", 5.4621), p("2026-11-06", 5.5013)]);
    assert.ok(r);
    assert.equal(r.previousDate, "2026-11-05");
    assert.equal(formatNumber(r.percent, 2), "0,72");
    assert.equal(lastChangePercent([p("2026-11-06", 5.5)]), null);
  });

  test("lastLevelChange acha a última mudança de patamar", () => {
    const series = [
      p("2026-07-30", 13.5),
      p("2026-07-31", 13.75),
      p("2026-08-01", 13.75),
      p("2026-08-02", 13.75),
    ];
    assert.deepEqual(lastLevelChange(series), { date: "2026-07-31", delta: 0.25 });
    assert.equal(lastLevelChange([p("2026-01-01", 10), p("2026-01-02", 10)]), null);
  });

  test("accumulated12Months compõe as variações", () => {
    // 1% ao mês por 12 meses: (1,01)^12 − 1 = 12,68%, e não 12%
    assert.equal(formatNumber(accumulated12Months(months(1))!, 2), "12,68");
  });

  test("accumulated12Months exige 12 meses seguidos", () => {
    assert.equal(accumulated12Months(months(1).slice(1)), null);
    const withGap = months(1);
    withGap[5] = p("2027-06-01", 1);
    assert.equal(accumulated12Months(withGap), null);
  });
});

describe("cards", () => {
  test("Selic mostra a última decisão", () => {
    const card = buildCard(indicator("selic", "Meta Selic", p("2026-11-07", 13.75)), [
      p("2026-09-17", 13.5),
      p("2026-09-18", 13.75),
      p("2026-11-07", 13.75),
    ]);
    assert.deepEqual(card, {
      id: "selic",
      label: "Meta Selic",
      value: "13,75%",
      context: "ao ano em 07/11/2026",
      detail: "+0,25 p.p. desde 18/09/2026",
      direction: "up",
    });
  });

  test("IPCA mostra o acumulado em 12 meses", () => {
    const card = buildCard(indicator("ipca", "IPCA", p("2026-09-01", 1)), months(1));
    assert.equal(card.value, "1,00%");
    assert.equal(card.context, "em set/2026");
    assert.equal(card.detail, "12,68% em 12 meses");
    assert.equal(card.direction, null);
  });

  test("Dólar mostra a variação no dia", () => {
    const card = buildCard(indicator("usd_brl", "Dólar", p("2026-11-06", 5.5013)), [
      p("2026-11-05", 5.4621),
      p("2026-11-06", 5.5013),
    ]);
    assert.equal(card.value, "R$ 5,5013");
    assert.equal(card.detail, "+0,72% sobre 05/11/2026");
    assert.equal(card.direction, "up");
  });

  test("sem histórico suficiente, o card fica sem detalhe", () => {
    const card = buildCard(indicator("usd_brl", "Dólar", p("2026-11-06", 5.5)), [p("2026-11-06", 5.5)]);
    assert.equal(card.detail, null);
    assert.equal(card.direction, null);
  });
});

describe("período", () => {
  test("periodStart volta N anos", () => {
    assert.equal(periodStart("2026-11-07", 5, "2000-01-01"), "2021-11-07");
    assert.equal(periodStart("2026-11-07", null, "2000-01-01"), "2000-01-01");
  });

  test("29 de fevereiro menos um ano vira 28", () => {
    assert.equal(periodStart("2024-02-29", 1, "2000-01-01"), "2023-02-28");
  });
});
