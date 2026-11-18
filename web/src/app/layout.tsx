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
        <header className="border-b border-hairline">
          <div className="mx-auto flex w-full max-w-5xl flex-wrap items-center justify-between gap-x-6 gap-y-2 px-4 py-3">
            <Link href="/" className="font-semibold tracking-tight text-ink">
              Radar Econômico
            </Link>
            <SiteNav />
          </div>
        </header>
        {children}
      </body>
    </html>
  );
}
