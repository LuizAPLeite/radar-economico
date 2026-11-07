# Radar Econômico

Projeto de portfólio full stack + dados. Coleta dados públicos brasileiros
(preços de combustíveis da ANP e indicadores do Banco Central e IBGE),
processa em camadas e exibe num dashboard web com gráficos e mapa.

## Objetivo
Demonstrar para recrutadores domínio de engenharia de dados, back-end,
front-end e deploy em um sistema real, de ponta a ponta, rodando em produção.
Qualidade e clareza importam mais que quantidade de features.

## Stack e responsabilidades
- **collector/** – Go. Coleta concorrente das fontes (goroutines), com retry,
  timeout e logs estruturados (slog). Grava os dados BRUTOS no schema `raw`.
  Não transforma dados.
- **pipeline/** – Python. Lê `raw`, limpa, padroniza e valida (pandera),
  grava em `staging` e `mart`. Também faz as análises (previsões, correlações).
  Usar polars de preferência.
- **api/** – Go. API REST somente leitura sobre o schema `mart`.
  chi (rotas), pgx + sqlc (acesso ao banco), cache no Redis, spec OpenAPI.
- **web/** – Next.js + TypeScript + Tailwind. Gráficos (ECharts ou Recharts)
  e mapa (MapLibre).
- **db/migrations/** – schemas `raw`, `staging`, `mart` no PostgreSQL.
- Infra: Docker Compose para desenvolvimento local; GitHub Actions para CI
  e para a coleta agendada.
- Deploy alvo: front na Vercel, API no Fly.io ou Railway, banco no Neon ou Supabase.

## Decisões tomadas
- Monorepo com `go.work` na raiz unindo os módulos `collector` e `api`
  (rodar `go test ./collector/... ./api/...` da raiz).
- Migrations com golang-migrate (SQL puro, pares up/down), aplicadas pelo
  serviço `migrate` do Compose; CI testa up → down -all → up.
- Python gerenciado com uv (`pipeline/uv.lock` versionado); lint/format com ruff.
- Front gerado com create-next-app (App Router, `src/`); CI roda lint,
  typecheck e build.
- Branch principal: `main`.
- raw do BCB: `raw.collection_runs` (uma linha por execução) e
  `raw.bcb_sgs_responses` (corpo JSON da API intacto, append-only).
  Séries do MVP: 432 (meta Selic), 433 (IPCA), 1 (dólar venda).
- Collector: séries diárias em janelas de até 10 anos desde `SGS_START_DATE`;
  4 consultas simultâneas (`errgroup.SetLimit`); uma falha não interrompe as
  demais. Imagem distroless.
- Testes de integração Go usam `TEST_DATABASE_URL` e são pulados sem ela;
  no CI o job Go sobe um Postgres e aplica as migrations antes.
- Pipeline BCB: staging e mart são reconstruídas a cada execução a partir da
  coleta bem-sucedida mais recente (que traz o histórico completo), com
  DELETE + COPY + ANALYZE numa transação. Cargas incrementais: Fase 3.
- Validação pandera ANTES de gravar: contrato violado = job falha e a mart
  fica intacta. Datas repetidas falham (não são deduplicadas em silêncio).
- mart: `mart.indicators` (ids `selic`, `ipca`, `usd_brl`, com limites de
  sanidade em `pipeline/.../bcb/indicators.py`) e `mart.indicator_values`
  (numeric). Datas após o dia da coleta (horário de Brasília) não entram.
- Testes de integração Python usam a mesma `TEST_DATABASE_URL` e rodam numa
  transação com `force_rollback`: não deixam rastro no banco.

## Regra de arquitetura
Coletor e pipeline NÃO se chamam diretamente: eles se comunicam pelo banco
(raw → staging → mart). A API só lê `mart`.

## Roteiro
- Fase 0: monorepo, Docker Compose (Postgres + Redis), migrations, CI básico.
- Fase 1: Banco Central (Selic, IPCA, dólar) de ponta a ponta, com deploy. ← MVP
- Fase 2: ANP (combustíveis) + filtros + mapa por município.
- Fase 3: cargas incrementais, testes, validação de dados, cache, coleta agendada,
  endpoint de saúde dos dados.
- Fase 4: análises (previsão de preços, correlação dólar × gasolina).
- Fase 5: assistente com LLM (pergunta em português → SQL → resposta + gráfico).
- Fase 6: README caprichado, diagrama, GIF de demo.
Não avançar de fase sem a anterior estar funcionando.

## Como trabalhamos juntos
- Estou aprendendo Go; explique as decisões e os padrões idiomáticos quando
  aparecerem pela primeira vez, de forma breve.
- Trabalhe em passos pequenos e verificáveis. Ao final de cada passo, diga
  como eu testo (comando para rodar e resultado esperado).
- Antes de mudanças grandes (nova dependência, mudança de estrutura ou de schema),
  proponha e espere meu ok.
- De vez em quando, deixe partes menores para eu implementar e depois revise
  meu código.
- Antes de escrever um coletor, confira o formato atual da fonte de dados
  (endpoints, layout dos CSVs), porque eles mudam.
- Commits pequenos, mensagens no padrão Conventional Commits, em português.
- Código e nomes de variáveis em inglês; README e documentação em português.

## Padrões de qualidade
- Testes para lógica de transformação, validação e endpoints da API.
- Sem credenciais no código: usar .env (com .env.example versionado).
- Todo componente deve subir com `docker compose up`.
- Mantenha este arquivo atualizado quando decisões importantes forem tomadas.