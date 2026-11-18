"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const LINKS = [
  { href: "/", label: "Indicadores" },
  { href: "/combustiveis", label: "Combustíveis" },
  { href: "/analises", label: "Análises" },
  { href: "/assistente", label: "Assistente" },
] as const;

/**
 * Links das páginas, com a atual marcada (aria-current). É Client Component
 * porque só o navegador sabe a URL atual: o layout não roda de novo a cada
 * navegação (ele é mantido entre as páginas).
 */
export function SiteNav() {
  const pathname = usePathname();
  return (
    <nav aria-label="Páginas">
      {/* flex-wrap: no celular, os links que não cabem descem para a linha de baixo. */}
      <ul className="flex flex-wrap gap-x-3">
        {LINKS.map(({ href, label }) => {
          const current = href === "/" ? pathname === "/" : pathname.startsWith(href);
          return (
            <li key={href}>
              <Link
                href={href}
                aria-current={current ? "page" : undefined}
                // A página atual ganha um fio grosso embaixo; as outras, um fio
                // fino ao passar o mouse.
                className={
                  "block border-b-2 px-2 py-1.5 text-sm transition-colors " +
                  (current
                    ? "border-accent font-medium text-ink"
                    : "border-transparent text-ink-2 hover:border-hairline hover:text-ink")
                }
              >
                {label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
