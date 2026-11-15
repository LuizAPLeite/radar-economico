import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";

// Content Security Policy: a lista do que a página pode carregar. Tudo vem do
// próprio site (o navegador nunca fala com a API nem com outro domínio).
// 'unsafe-inline' em script-src: o Next injeta scripts inline e a
// alternativa (nonce) obriga toda página a ser gerada a cada visita; risco
// aceito no relatório (docs/security/relatorio.md), já que o React escapa o
// texto e o tooltip do assistente também. 'unsafe-eval' só no `next dev`.
const csp = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline'${isDev ? " 'unsafe-eval'" : ""}`,
  "style-src 'self' 'unsafe-inline'", // ECharts e MapLibre usam estilo inline
  "img-src 'self' blob: data:",
  "font-src 'self'",
  "connect-src 'self'",
  "worker-src 'self' blob:", // worker do MapLibre (public/vendor)
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "upgrade-insecure-requests",
].join("; ");

const securityHeaders = [
  { key: "Content-Security-Policy", value: csp },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
  { key: "Strict-Transport-Security", value: "max-age=31536000" },
];

const nextConfig: NextConfig = {
  // Gera .next/standalone: só os arquivos necessários para rodar em produção,
  // o que deixa a imagem Docker bem menor (ver web/Dockerfile).
  output: "standalone",
  poweredByHeader: false, // não anuncia "X-Powered-By: Next.js"
  async headers() {
    return [{ source: "/(.*)", headers: securityHeaders }];
  },
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
