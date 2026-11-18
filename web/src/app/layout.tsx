import type { Metadata } from "next";
import { Archivo } from "next/font/google";
import Link from "next/link";

import { SiteNav } from "@/components/SiteNav";
import "./globals.css";

// Uma família só, com eixo de largura: o texto usa a largura normal; os
// números grandes, a estreita, como nos totens de preço dos postos. O
// next/font baixa a fonte no build e a serve do próprio site (CSP 'self').
const archivo = Archivo({
  variable: "--font-archivo",
  subsets: ["latin"],
  axes: ["wdth"],
});

export const metadata: Metadata = {
  // Cada página dá o próprio título ("Combustíveis"), e o template completa.
  title: { default: "Radar Econômico", template: "%s · Radar Econômico" },
  description: "Indicadores econômicos e preços de combustíveis do Brasil",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="pt-BR"
      className={`${archivo.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        {/* Para quem navega pelo teclado: o primeiro Tab oferece pular o menu. */}
        <a
          href="#conteudo"
          className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-sm focus:bg-ink focus:px-3 focus:py-2 focus:text-page"
        >
          Pular para o conteúdo
        </a>
        <header className="border-b border-hairline">
          <div className="mx-auto flex w-full max-w-5xl flex-wrap items-center justify-between gap-x-6 gap-y-2 px-4 py-3">
            <Link href="/" className="font-semibold tracking-tight text-ink">
              Radar Econômico
            </Link>
            <SiteNav />
          </div>
        </header>
        <div id="conteudo" tabIndex={-1} className="flex flex-1 flex-col outline-none">
          {children}
        </div>
        <footer className="border-t border-hairline">
          <nav
            aria-label="Rodapé"
            className="mx-auto flex w-full max-w-5xl flex-wrap gap-x-6 gap-y-2 px-4 py-5 text-sm text-ink-2"
          >
            <Link href="/privacidade" className="hover:text-ink">
              Privacidade
            </Link>
            <Link href="/termos" className="hover:text-ink">
              Termos de uso
            </Link>
            <a href="https://github.com/LuizAPLeite/radar-economico" className="hover:text-ink">
              Código no GitHub
            </a>
            <span className="text-muted">Projeto pessoal, sem cookies nem rastreamento.</span>
          </nav>
        </footer>
      </body>
    </html>
  );
}
