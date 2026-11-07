import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Gera .next/standalone: só os arquivos necessários para rodar em produção,
  // o que deixa a imagem Docker bem menor (ver web/Dockerfile).
  output: "standalone",
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
