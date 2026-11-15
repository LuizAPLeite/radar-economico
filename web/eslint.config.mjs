import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import security from "eslint-plugin-security";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Regras de segurança (eval, regex lentas, require dinâmico...).
  security.configs.recommended,
  {
    rules: {
      // Acusa todo obj[chave] (45 avisos, todos com chaves tipadas pelo
      // TypeScript ou vindas de listas fixas): ruído, não sinal. Poluição de
      // protótipo fica com o CodeQL. Ver docs/security/relatorio.md.
      "security/detect-object-injection": "off",
    },
  },
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // Worker do MapLibre copiado de node_modules (scripts/copy-maplibre-worker.mjs).
    "public/vendor/**",
  ]),
]);

export default eslintConfig;
