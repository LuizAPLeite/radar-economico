"use client";

import { useState } from "react";

import type { MunicipalPrice } from "@/lib/api";
import { formatPrice } from "@/lib/fuels";
import { formatMonth, formatNumber } from "@/lib/indicators";

/**
 * Todos os municípios do mês em tabela: o equivalente acessível do mapa, e o
 * jeito de achar uma cidade pelo nome. Vem em ordem de UF e nome, como a API.
 *
 * As linhas só são montadas quando a pessoa abre a tabela: com ~400
 * municípios, deixá-las prontas no HTML pesaria a página para todo mundo.
 * Os dados já chegam ao navegador de qualquer jeito, para o mapa.
 */
export function MunicipalTable({ data, month }: { data: MunicipalPrice[]; month: string | null }) {
  const [open, setOpen] = useState(false);
  if (data.length === 0 || !month) return null;

  return (
    <details
      onToggle={(e) => setOpen(e.currentTarget.open)}
      className="mt-4 rounded-xl border border-hairline bg-surface p-5 text-sm"
    >
      <summary className="cursor-pointer text-ink-2 hover:text-ink">
        Ver os {formatNumber(data.length, 0)} municípios de {formatMonth(month)} em tabela
      </summary>
      {open && (
        <div className="mt-3 overflow-x-auto">
          <table className="w-full min-w-[32rem] tabular-nums">
            <thead>
              <tr className="border-b border-hairline text-left text-ink-2">
                <th className="py-1.5 font-medium">Município</th>
                <th className="py-1.5 font-medium">UF</th>
                <th className="py-1.5 text-right font-medium">Preço médio</th>
                <th className="py-1.5 text-right font-medium">Menor e maior</th>
                <th className="py-1.5 text-right font-medium">Postos</th>
              </tr>
            </thead>
            <tbody>
              {data.map((m) => (
                <tr key={m.ibge_code} className="border-b border-hairline last:border-0">
                  <td className="py-1.5 text-ink">{m.name}</td>
                  <td className="py-1.5 text-ink-2">{m.uf}</td>
                  <td className="py-1.5 text-right text-ink">{formatPrice(m.avg_price)}</td>
                  <td className="py-1.5 text-right text-ink-2">
                    {m.n_stations === 1 ? "—" : `${formatPrice(m.min_price)} a ${formatPrice(m.max_price)}`}
                  </td>
                  <td className="py-1.5 text-right text-ink-2">{m.n_stations}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </details>
  );
}
