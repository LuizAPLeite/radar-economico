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
- API: chi + pgx + sqlc. Queries em `api/queries/`, código gerado em
  `api/internal/db` com sqlc via Docker (`sqlc/sqlc:1.31.1`); CI roda
  `sqlc diff`. numeric → float64 na API; datas como `AAAA-MM-DD`.
- API conecta com `default_transaction_read_only=on`: o Postgres recusa escrita.
- Rotas: `/health`, `/v1/indicators`, `/v1/indicators/{id}/values?from&to`,
  `/openapi.yaml` (spec embutida com go:embed). Erros sempre
  `{"error":{"code","message"}}`; 500 nunca expõe o erro interno.
- Cache Redis na API fica para a Fase 3.
- Web: Next.js 16 com Cache Components (ler a doc em
  `web/node_modules/next/dist/docs/` antes de mexer: a API mudou). Dados
  buscados no servidor (Server Component dentro de `<Suspense>`), sem cache
  no Next por enquanto; `next build` não chama a API. URL em `API_URL`
  (runtime; no Compose `http://api:8080`). Navegador nunca fala com a API.
- Web: gráficos com echarts + echarts-for-react (import modular); cores como
  variáveis CSS em `globals.css`, da paleta do guia de dataviz, validadas com
  o script dele nos dois modos. Um seletor de período único para todos os
  gráficos (sem zoom por gráfico); tabela acessível ao lado de cada gráfico.
- Web: testes de funções puras com `node --test` (Node 26 roda TS direto,
  imports com extensão `.ts`); imagem Docker com `output: "standalone"`.
- Deploy ADIADO por decisão de custo (nov/2026): a configuração está pronta,
  mas nada foi publicado. Publicar quando o usuário decidir; Neon e Vercel são
  grátis, só a API no Fly.io custa (~US$ 2/mês, exige cartão). Revisar os
  preços na hora, ou considerar um host gratuito para a API.
- Deploy planejado (tudo em São Paulo): Neon Free (Postgres 16, `sa-east-1`),
  API no Fly.io (`gru`, auto-stop, menor máquina), web na Vercel Hobby (`gru1`,
  raiz `web`). Coleta em produção pelo workflow manual `data.yml`.
- Neon: usar a connection string DIRETA (sem `-pooler`): o pooler recusa o
  parâmetro `default_transaction_read_only` que a API envia ao conectar.
  O Neon dorme após 5 min sem consultas e derruba as conexões, por isso o
  pool da API fecha conexões ociosas após 1 min.
- Plano grátis do Neon tem 100 CU-horas/mês: antes de publicar, a API precisa
  de cache em memória (`api/internal/cache`) para não acordar o banco a cada visita.

- Collector com várias fontes (`bcb_sgs`, `ibge`, `anp`) em goroutines, cada
  uma com sua linha em `raw.collection_runs`; `COLLECTOR_SOURCES` escolhe
  quais rodar. Pacote `fetch` (GET com retry/backoff) compartilhado.
- IBGE (`raw.ibge_responses`) e ANP (`raw.anp_files`) só gravam quando o
  SHA-256 do conteúdo muda. Arquivos da ANP guardados intactos, compactados
  com gzip (coluna com STORAGE EXTERNAL, sem recompressão do Postgres).
- ANP: os nomes dos arquivos mensais não seguem padrão e meses podem faltar;
  o collector descobre os links lendo a página de dados abertos. O servidor
  recusa HEAD (403) e é lento (~200 KB/s): só GET, 2 downloads por vez,
  coleta incremental (só baixa meses novos + os mais recentes).

- Pipeline com três jobs independentes (`bcb`, `ibge`, `anp`, nessa ordem;
  `PIPELINE_JOBS` escolhe): um que falha não impede os outros. O job da ANP
  lê os municípios que o do IBGE gravou em `staging.municipalities`.
- `pg.copy_frame`: COPY em formato CSV (polars `write_csv`), compartilhado.
  Staging grande (`staging.fuel_prices`) é esvaziado com TRUNCATE; a mart,
  lida pela API, com DELETE (MVCC). `mart.municipalities` é atualizada com
  upsert (é referenciada por chave estrangeira).
- ANP no pipeline: produto ou unidade fora do catálogo (`anp/products.py`) e
  município sem par no IBGE fazem o job falhar; exceções de nome em
  `MUNICIPALITY_EXCEPTIONS` (`anp/transform.py`). Cópias idênticas (mesmo
  posto, produto, dia e preço) são removidas; preços diferentes no mesmo
  dia falham.
- Média semanal: semana começa na segunda; semanas com menos de 50% da
  mediana de postos do produto (ex.: bordas de um mês que a ANP não
  publicou) saem do Brasil e de todas as UFs.

## Regra de arquitetura
Coletor e pipeline NÃO se chamam diretamente: eles se comunicam pelo banco
(raw → staging → mart). A API só lê `mart`.

## Roteiro
- Fase 0: monorepo, Docker Compose (Postgres + Redis), migrations, CI básico.
- Fase 1: Banco Central (Selic, IPCA, dólar) de ponta a ponta, com deploy. ← MVP
  Concluída localmente; o deploy ficou para depois (ver "Decisões tomadas").
- Fase 2: ANP (combustíveis) + filtros + mapa por município. ← atual
- Fase 3: cargas incrementais, testes, validação de dados, cache, coleta agendada,
  endpoint de saúde dos dados.
- Fase 4: análises (previsão de preços, correlação dólar × gasolina).
- Fase 5: assistente com LLM (pergunta em português → SQL → resposta + gráfico).
- Fase 6: README caprichado, diagrama, GIF de demo.
Não avançar de fase sem a anterior estar funcionando (exceção combinada: o
deploy da Fase 1 fica para quando o usuário decidir gastar).

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