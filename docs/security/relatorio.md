# Relatório de segurança: antes e depois

Etapa de DevSecOps do Radar Econômico (novembro de 2026). Todas as
varreduras rodaram **só contra o próprio projeto**: o código deste
repositório, os containers locais (Docker Compose) e o nosso deploy.

O ponto de partida foi o [modelo de ameaças](threat-model.md), que lista os
achados `AM-xx` encontrados lendo a arquitetura. Depois vieram as
ferramentas automáticas. Cada achado foi registrado antes da correção, e os
números do "antes" estão nas mensagens de commit.

## Resumo

| Fase | Ferramenta | Antes | Depois |
|---|---|---|---|
| 1. Modelo de ameaças | STRIDE, manual | 14 achados (2 altos, 5 médios) | 2 altos corrigidos (AM-02 em parte); médios corrigidos; ver a tabela abaixo |
| 2. Segredos | gitleaks (124 commits) | 0 vazamentos | 0, com pre-commit e CI |
| 3. SAST Go | govulncheck | 1 vulnerabilidade (GO-2026-6859, `net/http`) | 0 (Go 1.27.4) |
| 3. SAST Go | gosec | 1 alto (G404, `math/rand`) | 0 (falso positivo, `#nosec` justificado) |
| 3. SAST Python | bandit | 1 médio (B608, SQL com f-string) | 0 (`psycopg.sql.Identifier`) |
| 3. SAST Python | pip-audit | 0 | 0 |
| 3. SAST Web | npm audit | 7 moderadas (só desenvolvimento) | 0 (override do `minimatch`) |
| 3. SAST Web | eslint-plugin-security | 46 avisos | 0 (45 eram uma regra ruidosa, desligada; 1 falso positivo) |
| 3. SAST | CodeQL (Go, Python, TS) | em Security → Code scanning | ver o painel do GitHub |
| 4. Containers | Trivy (imagens) | 13 altas (pipeline 7, web 6) | 0 |
| 4. Containers | Trivy (configuração) | 2 altas (USER nos Dockerfiles Go) | 0 |
| 6. DAST | OWASP ZAP (web) | 0 FAIL, 5 tipos de aviso | 0 avisos novos (5 aceitos, abaixo) |
| 6. DAST | OWASP ZAP (API, pela OpenAPI) | 0 FAIL, 1 aviso | 0 |

## O achado mais grave: AM-01

O assistente transforma uma pergunta em SQL, e o SQL roda num papel do
banco que só lê tabelas públicas. Num banco isolado, simulando o Neon,
mostramos que dava para **sair desse papel**: `set_config('role', ...)`
voltava ao usuário da conexão, que era o dono do banco, e `query_to_xml`
executava outra consulta com esse poder. Assim se liam as mensagens de
erro internas e o schema `raw`. A correção (usuário `radar_api` que só lê a
mart, funções perigosas barradas no `CheckSQL` e o canário diário rodando a
API com as permissões de produção) foi conferida no banco de produção. O
documento só foi publicado junto com a correção. Detalhes e a tabela de
antes e depois estão no [modelo de ameaças](threat-model.md#am-01-fuga-do-papel-radar_assistant-alto-confirmado).

## Achados do modelo de ameaças

| ID | Risco | Situação | O que foi feito |
|---|---|---|---|
| AM-01 | Alto | **Corrigido** | `radar_api` (migration 000009), `CheckSQL` com funções barradas, canário como `radar_api` |
| AM-02 | Alto | **Em parte** | A API (a parte exposta) já não usa a credencial de dono. Coleta e pipeline ainda usam: ver "Riscos aceitos" |
| AM-03 | Médio | **Corrigido** | Limite por IP no `POST /v1/ask` (10 por minuto, `429 rate_limited` com `Retry-After`), além do limite diário |
| AM-04 | Médio | **Corrigido** | Tooltip do assistente escapa o HTML (`escapeHtml`, com teste) |
| AM-05 | Médio | **Corrigido** | CSP e cabeçalhos no web (`next.config.ts`) e na API (`security.go`) |
| AM-06 | Médio | **Corrigido** | Actions fixadas pelo SHA do commit; no `data.yml` a credencial só entra nos passos que falam com o banco |
| AM-07 | Médio | **Corrigido** | `permissions: contents: read` em todos os workflows |
| AM-08 | Baixo | **Corrigido** | Coletor da ANP só segue `https://www.gov.br/...` |
| AM-09 | Baixo | Aceito | Ver abaixo |
| AM-10 | Baixo | **Corrigido** | Aviso na página do assistente sobre o plano grátis do Gemini |
| AM-11 | Baixo | Aceito | Ver abaixo |
| AM-12 | Baixo | Aceito | Ver abaixo |
| AM-13 | A verificar | Checklist | Ver "Contas" abaixo |
| AM-14 | Baixo | **Corrigido** | Dependabot (Go, uv, npm, Docker, Actions), Trivy toda semana, versões de build fixadas |

## Riscos aceitos, e por quê

- **AM-02, coleta e pipeline com a credencial de dono.** Rodam só no
  GitHub Actions, sem entrada vinda da internet, e o segredo só chega aos
  passos que falam com o banco (AM-06). Um usuário por componente exige
  dois segredos novos e uma migration de papéis. Fica como próximo passo,
  no mesmo padrão do `radar_api`.
- **AM-09, sem limite de descompressão.** Os arquivos vêm só de
  `https://www.gov.br` (AM-08) e de dois endereços fixos (série da ANP e
  FRED). O download já tem limite de 100 MB, e o processo roda no Actions,
  isolado e com tempo máximo.
- **AM-11, `/health/data` público.** A transparência é intencional: mostra
  idade dos dados e avisos de qualidade, nunca mensagens de erro (a coluna
  `error` nem é legível pelo `radar_api`).
- **AM-12, credencial de dono na máquina local.** Fica fora do git (o
  gitleaks confere a cada commit) e é a única forma de aplicar migrations
  no Neon de fora do Actions.
- **CSP com `'unsafe-inline'` em `script-src` (ZAP 10055).** O Next injeta
  scripts inline. A alternativa, um nonce por requisição, obriga cada página
  a ser gerada a cada visita e anula o pré-render. Mitigação: o React
  escapa todo texto, o único HTML montado à mão (o tooltip) é escapado, e a
  CSP ainda bloqueia scripts de outros domínios, `object`, `base` e o site
  dentro de iframes.
- **Sem Cross-Origin-Embedder-Policy (ZAP 90004).** O site não usa
  recursos que exigem isolamento entre origens. COOP e CORP estão
  definidos.
- **Falsos positivos do ZAP no formulário do assistente (10202, 10031).**
  É uma Server Action. O Next recusa POST cuja origem não seja o próprio
  site, e os campos acusados são ids internos gerados pelo framework.
- **`security/detect-object-injection` desligada.** Acusava todo
  `obj[chave]` (45 avisos, todos com chaves tipadas ou de listas fixas). O
  CodeQL cobre poluição de protótipo com análise de fluxo de verdade.
- **Limite por IP atrás de proxies.** O site chama a API pelo servidor da
  Vercel, então as perguntas feitas pelo site dividem os IPs da Vercel.
  Com o tráfego atual isso não pesa, e o limite diário continua valendo
  para todos.

## Contas (AM-13)

Não dá para verificar pelo código. Checklist do mantenedor:

- [ ] 2FA no GitHub, na Vercel, no Render, no Neon, no Google (Gemini) e no UptimeRobot.
- [ ] Proteção da branch `main` no GitHub (exigir o CI verde antes do merge).
- [ ] Alertas do Dependabot e do Code scanning ligados em Settings → Code security.
- [ ] Reporte privado de vulnerabilidades ligado (Settings → Code security → Private vulnerability reporting), usado pelo `SECURITY.md`.

## Como o pipeline de segurança funciona agora

| Onde | O quê | Quando falha |
|---|---|---|
| Máquina local | gitleaks no pre-commit (`uvx pre-commit install`) | Segredo no commit |
| `security.yml` | gitleaks (histórico), gosec, govulncheck, bandit, pip-audit, npm audit, Trivy (imagens e configuração), ZAP (web e API) | Achado crítico ou alto; aviso novo do ZAP |
| `codeql.yml` | CodeQL `security-extended` em Go, Python e TypeScript | Resultados em Security → Code scanning |
| `ci.yml` | Lint com regras de segurança (`--max-warnings 0`), testes de `CheckSQL`, limite por IP e cabeçalhos | Qualquer aviso ou teste |
| `fontes.yml` | API rodando como `radar_api`, com as permissões de produção | Permissão faltando numa tabela da mart |
| Dependabot | PRs semanais de Go, Python, npm, Docker e Actions | O CI de cada PR |

O `security.yml` roda a cada push e PR e toda segunda-feira, porque
vulnerabilidades novas aparecem em dependências que não mudaram.
