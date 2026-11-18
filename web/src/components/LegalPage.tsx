import type { ReactNode } from "react";

/** Moldura das páginas de texto (privacidade, termos): título, data e prosa legível. */
export function LegalPage({ title, updated, children }: { title: string; updated: string; children: ReactNode }) {
  return (
    <main className="mx-auto w-full max-w-2xl flex-1 px-4 py-12">
      <h1 className="text-3xl font-semibold tracking-tight text-ink">{title}</h1>
      <p className="mt-2 text-sm text-ink-2">Última atualização: {updated}</p>
      <div
        className={
          "mt-8 space-y-4 leading-relaxed text-ink-2 " +
          "[&_a]:text-ink [&_a]:underline [&_a]:underline-offset-4 hover:[&_a]:no-underline " +
          "[&_h2]:pt-6 [&_h2]:text-xl [&_h2]:font-semibold [&_h2]:text-ink " +
          "[&_li]:ml-5 [&_li]:list-disc [&_li]:pl-1 [&_strong]:font-medium [&_strong]:text-ink"
        }
      >
        {children}
      </div>
    </main>
  );
}
