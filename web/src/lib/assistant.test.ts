import assert from "node:assert/strict";
import { describe, test } from "node:test";

import type { Answer } from "./api.ts";
import {
  chartData,
  columnLabel,
  dateGranularity,
  formatDateAs,
  errorMessage,
  EXAMPLES,
  formatCell,
  MAX_QUESTION,
  validateQuestion,
} from "./assistant.ts";

const answer = (overrides: Partial<Answer>): Answer => ({
  question: "q",
  answerable: true,
  title: "t",
  explanation: "e",
  sql: "SELECT 1",
  chart: { type: "line", x: "mes", y: ["preco", "postos"] },
  columns: [
    { name: "mes", type: "date" },
    { name: "uf", type: "text" },
    { name: "preco", type: "number" },
    { name: "postos", type: "number" },
  ],
  rows: [
    ["2026-09-01", "SP", 6.4, 900],
    ["2026-10-01", "SP", null, 910],
  ],
  truncated: false,
  cached: false,
  ...overrides,
});

describe("pergunta", () => {
  test("limites em caracteres, não em bytes", () => {
    assert.equal(validateQuestion("  "), "Escreva uma pergunta.");
    assert.equal(validateQuestion("ab"), "Escreva uma pergunta.");
    assert.equal(validateQuestion("abc"), null);
    assert.equal(validateQuestion("é".repeat(MAX_QUESTION)), null); // 600 bytes, 300 caracteres
    assert.match(validateQuestion("a".repeat(MAX_QUESTION + 1)) ?? "", /até 300/);
  });

  test("os exemplos são perguntas válidas", () => {
    for (const q of EXAMPLES) assert.equal(validateQuestion(q), null, q);
  });

  test("mensagem por código de erro, com padrão para o desconhecido", () => {
    assert.match(errorMessage("assistant_limit"), /limite/);
    assert.match(errorMessage("unanswerable"), /reformular/);
    assert.match(errorMessage("http_500"), /indisponível/);
  });
});

describe("datas", () => {
  test("granularidade pela forma das datas", () => {
    assert.equal(dateGranularity(["2015-01-01", "2016-01-01"]), "year");
    assert.equal(dateGranularity(["2026-01-01", "2026-02-01"]), "month");
    assert.equal(dateGranularity(["2026-01-01", "2026-01-02"]), "day");
    assert.equal(dateGranularity([]), "day");
  });

  test("formato por granularidade", () => {
    assert.equal(formatDateAs("2015-01-01", "year"), "2015");
    assert.equal(formatDateAs("2026-09-01", "month"), "set/2026");
    assert.equal(formatDateAs("2026-09-15", "day"), "15/09/2026");
    assert.equal(formatCell("2026-09-01", "date", "month"), "set/2026");
  });
});

describe("tabela", () => {
  test("células por tipo", () => {
    assert.equal(formatCell("2026-11-02", "date"), "02/11/2026");
    assert.equal(formatCell(6.473, "number"), "6,473");
    assert.equal(formatCell(1234.5, "number"), "1.234,5");
    assert.equal(formatCell(null, "number"), "—");
    assert.equal(formatCell("SP", "text"), "SP");
    assert.equal(formatCell(true, "text"), "sim");
  });

  test("rótulo da coluna", () => {
    assert.equal(columnLabel("preco_medio"), "Preco medio");
    assert.equal(columnLabel("uf"), "Uf");
  });
});

describe("gráfico", () => {
  test("separa x e séries, com null onde falta número", () => {
    const a = answer({
      chart: { type: "line", x: "mes", y: ["preco", "postos"] },
      rows: [
        ["2026-09-01", "SP", 6.4, 6.2],
        ["2026-10-01", "SP", null, 6.3],
      ],
    });
    assert.deepEqual(chartData(a), {
      kind: "line",
      xType: "date",
      granularity: "month",
      x: ["2026-09-01", "2026-10-01"],
      series: [
        { name: "Preco", values: [6.4, null] },
        { name: "Postos", values: [6.2, 6.3] },
      ],
      omitted: [],
    });
  });

  test("série noutra escala fica só na tabela (um eixo só)", () => {
    // dólar (~5) e Brent (~100): no mesmo eixo, o dólar seria uma linha achatada.
    const data = chartData(answer({}));
    assert.deepEqual(
      data?.series.map((s) => s.name),
      ["Preco"],
    );
    assert.deepEqual(data?.omitted, ["Postos"]);
  });

  test("no máximo duas séries", () => {
    const a = answer({ chart: { type: "line", x: "mes", y: ["preco", "postos", "preco"] } });
    assert.equal(chartData(a)?.series.length, 2);
  });

  test("sem gráfico quando não faz sentido", () => {
    assert.equal(chartData(answer({ chart: { type: "none", x: "", y: [] } })), null);
    assert.equal(chartData(answer({ rows: [] })), null);
    assert.equal(chartData(answer({ chart: { type: "bar", x: "nada", y: ["preco"] } })), null);
    assert.equal(chartData(answer({ chart: { type: "bar", x: "mes", y: ["uf"] } })), null); // y de texto
  });
});
