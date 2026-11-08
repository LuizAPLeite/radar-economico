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
  chi (rotas), pgx + sqlc (acesso ao banco), cache em memória, spec OpenAPI.
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
- Coleta do BCB incremental: cada série volta 30 dias (IPCA, mensal: 120)
  antes de até onde as coletas bem-sucedidas já foram (`store.BCBCoverage`:
  fim da janela, ou o dia da coleta nas consultas sem datas). A primeira
  coleta, ou `SGS_FULL_HISTORY=true`, traz o histórico completo. A API SGS
  aceita período também no IPCA e responde 404 em período vazio.
- Testes de integração Go usam `TEST_DATABASE_URL` e são pulados sem ela;
  no CI o job Go sobe um Postgres e aplica as migrations antes.
- Pipeline BCB incremental: processa as coletas bem-sucedidas depois da
  última já processada (`runs.last_source_run`: maior `source_run_id` com
  sucesso em `mart.pipeline_runs`); entre coletas, vale a mais recente para
  cada data; grava com upsert (`pg.upsert_frame`: COPY para tabela temporária
  + INSERT ... ON CONFLICT DO UPDATE ... WHERE IS DISTINCT FROM), sem apagar
  nada. `PIPELINE_FULL_REFRESH=true` relê todo o raw e reconstrói com
  DELETE + COPY + ANALYZE.
- ANP e IBGE: o job termina como skipped se o maior `run_id` do raw não
  passou do último processado; com novidade, reconstrói tudo (poucos
  segundos; reprocessar só o mês que mudou não compensa).
- Validação pandera ANTES de gravar: contrato violado = job falha e a mart
  fica intacta. Datas repetidas na mesma coleta falham (não são deduplicadas
  em silêncio).
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
  `/v1/fuels`, `/v1/fuels/{id}/prices?scope&from&to` (scope `BR` ou UF),
  `/v1/fuels/{id}/municipalities?month=AAAA-MM` (padrão: mês mais recente;
  traz `available_months`), `/openapi.yaml` (spec embutida com go:embed).
  Erros sempre `{"error":{"code","message"}}`; 500 nunca expõe o erro interno.
- `httpapi.Store` embute uma interface por recurso (`IndicatorStore`,
  `FuelStore`); nos testes, `fakeStore` embute `fakeFuelStore`.
- Cache em memória na API: `internal/cache` (TTL + mutex, genérico; escrito
  pelo usuário) e `internal/cachedstore`, um decorator de `httpapi.Store`.
  Séries guardadas inteiras por chave e recortadas por período em memória;
  erros (inclusive "não encontrado") não entram; listas saem copiadas. Mês de
  municípios fora de `available_months` responde vazio sem consultar o banco
  (limita as chaves). TTL em `CACHE_TTL`: padrão e produção 30m, Compose 1m,
  `0` desliga. Sem Redis: uma instância da API e dados que mudam uma vez por dia.
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
- Web, combustíveis (`/combustiveis`): filtros na URL (`combustivel`, `uf`,
  `mes`; valor inválido vira o padrão), trocados com `router.push` numa
  transição + `useOptimistic` (padrão da doc do Next 16): o conteúdo antigo
  esmaece, sem voltar ao esqueleto. Navegação entre páginas no layout
  (`SiteNav`); títulos com o template "%s · Radar Econômico".
- Mapa: MapLibre GL 6, só no cliente (`next/dynamic` com `ssr: false`), sem
  serviço de mapas: o fundo é o contorno das UFs do IBGE
  (`web/public/geo/ufs.json`, com `bbox` por UF, gerado por
  `web/scripts/build-ufs-geojson.mjs`). O worker do MapLibre 6 precisa ser
  servido: `predev`/`prebuild` copiam para `web/public/vendor/` (fora do git)
  e o mapa chama `setWorkerUrl`. Cores em 5 faixas por quintis (comparando em
  centavos, arredondados como na tela), uma escala só para o país; no escuro
  a escala inverte. A tabela com todos os municípios só é montada quando
  aberta (os dados já vêm para o mapa).
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
- Plano grátis do Neon tem 100 CU-horas/mês: por isso a API guarda as
  respostas em memória (ver o cache acima) e não acorda o banco a cada visita.

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
- Fase 3, plano aprovado (nov/2026): cache só em memória (o Redis sai do
  Compose); coleta agendada no `data.yml` desligada por variável até o deploy,
  mais um canário diário no CI (fontes reais num Postgres temporário);
  testes de contrato da API contra a `openapi.yaml` com kin-openapi.
- Cada execução de job vira uma linha em `mart.pipeline_runs` (success,
  skipped quando não há nada no raw, failed; coleta de origem, linhas
  gravadas, avisos). Carga e registro do sucesso na mesma transação (a do job
  vira SAVEPOINT); a falha é registrada depois do rollback. A mensagem de
  erro fica só no banco: a API não a expõe.
- Validação em dois níveis: contrato pandera (falha o job, mart intacta) e
  avisos de qualidade (`bcb/quality.py`, `anp/quality.py`, IBGE no job), que
  não impedem a carga e vão para `pipeline_runs.warnings`. Avisos só olham o
  que é NOVO na carga (valores/semanas depois do último que a mart tinha;
  primeira carga não checa), para não repetir o histórico. Limites
  calibrados nos dados reais: dólar 10%/dia, Selic e IPCA 2 p.p. (no
  catálogo `indicators.py`), média semanal 15%, mês < 70% da mediana do
  produto, UFs < mediana do produto − 2, mais de 10 municípios sem
  coordenadas. Mensagens em português, com números no padrão brasileiro.
- Média semanal: semana começa na segunda; semanas com menos de 50% da
  mediana de postos do produto (ex.: bordas de um mês que a ANP não
  publicou) saem do Brasil e de todas as UFs.

## Regra de arquitetura
Coletor e pipeline NÃO se chamam diretamente: eles se comunicam pelo banco
(raw → staging → mart). A API só lê `mart`.

## Roteiro
- Fase 0: monorepo, Docker Compose (Postgres; o Redis saiu na Fase 3), migrations, CI básico.
- Fase 1: Banco Central (Selic, IPCA, dólar) de ponta a ponta, com deploy. ← MVP
  Concluída localmente; o deploy ficou para depois (ver "Decisões tomadas").
- Fase 2: ANP (combustíveis) + filtros + mapa por município.
  Concluída localmente.
- Fase 3: cargas incrementais, testes, validação de dados, cache, coleta agendada,
  endpoint de saúde dos dados. ← atual
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