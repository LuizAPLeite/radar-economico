# Radar Econômico

Coleta dados públicos brasileiros (Banco Central, IBGE e ANP), processa em
camadas e exibe num dashboard web com gráficos e mapa.

> **Fases 1 e 2 concluídas:** Selic, IPCA e dólar do Banco Central, e os
> preços de combustíveis da ANP com mapa por município, de ponta a ponta,
> rodando localmente com `docker compose up` (o deploy está configurado, mas
> ainda não publicado). **Em andamento:** Fase 3, a operação dos dados:
> registro das execuções, cache, cargas incrementais, saúde dos dados e
> coleta agendada.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/combustiveis-escuro.png">
  <img alt="Página de combustíveis do Radar Econômico: filtros de combustível e estado, cards com a gasolina a R$ 6,51 no Brasil, o município mais barato (Goiatuba, R$ 5,74) e o mais caro (Parintins, R$ 8,99) em outubro de 2026, o gráfico do preço médio semanal e o mapa do Brasil com um ponto colorido por município pesquisado, ao lado dos rankings" src="docs/img/combustiveis.png">
</picture>

## Arquitetura

```
fontes públicas ──► collector (Go) ──► raw ──► pipeline (Python) ──► staging ──► mart ──► api (Go) ──► web (Next.js)
```

Coletor e pipeline não se chamam diretamente: comunicam-se pelo banco.
A API só lê o schema `mart`.

| Pasta | Stack | Papel |
|---|---|---|
| `collector/` | Go | Coleta concorrente das fontes, grava em `raw` |
| `pipeline/` | Python, polars, pandera | Limpa e valida `raw` → `staging` → `mart` |
| `api/` | Go | API REST somente leitura sobre `mart` |
| `web/` | Next.js, TypeScript, Tailwind, ECharts, MapLibre | Dashboard com gráficos e mapa |
| `db/migrations/` | SQL (golang-migrate) | Schemas e tabelas |

## Rodando localmente

Pré-requisitos: Docker, Go, Python 3.13 + [uv](https://docs.astral.sh/uv/), Node.js.

```bash
cp .env.example .env
docker compose up -d          # banco, migrations, coleta, pipeline, API e dashboard
docker compose exec postgres psql -U radar -c '\dn'   # deve listar raw, staging, mart
```

Além do Postgres, o `docker compose up` sobe:

1. **collector** (roda uma vez): coleta em paralelo o Banco Central (Selic,
   IPCA, dólar), o IBGE (municípios e coordenadas) e a ANP (preços de
   combustíveis por posto, últimos 12 meses) e grava tudo, intacto, no schema
   `raw`. As coletas são **incrementais**: a primeira traz o histórico (o BCB
   desde 2000; a primeira da ANP leva alguns minutos, porque o servidor é
   lento), e as seguintes só buscam o que pode ter mudado. No BCB, isso são
   as últimas semanas de cada série, com uma margem para valores publicados
   com atraso ou corrigidos: ~3 KB por coleta, em vez de ~100 KB;
2. **pipeline** (roda uma vez, depois do collector): três jobs, cada um lendo
   do `raw` só o que chegou desde a última execução, validando com pandera e
   gravando `staging` e `mart`:
   - **bcb**: indicadores (`mart.indicator_values`). Junta as coletas novas e
     grava com upsert (insere as datas novas, atualiza os valores corrigidos
     e não apaga nada);
   - **ibge**: municípios com coordenadas (`mart.municipalities`);
   - **anp**: ~780 mil preços por posto (`staging.fuel_prices`), casados com os
     municípios do IBGE, e as médias semanais por UF e Brasil
     (`mart.fuel_prices_weekly`) e mensais por município
     (`mart.fuel_prices_municipal`). Reconstrói tudo em uns 5 segundos quando
     chega arquivo novo.

   Sem nada novo no `raw`, o job termina na hora, sem tocar na mart.

   Cada execução de cada job fica registrada em `mart.pipeline_runs`: se deu
   certo, falhou ou não tinha nada novo, de qual coleta vieram os dados e
   quantas linhas foram gravadas. A carga e o registro do sucesso vão na
   mesma transação; se o job falha, a carga é desfeita e a falha é registrada.

   A validação tem dois níveis. O **contrato** (pandera) barra o que está
   claramente errado, e o job falha sem tocar na mart. Os **avisos** marcam o
   que pode ser erro, mas também pode ser real, e ficam no registro sem
   impedir a carga: um salto grande nos valores novos (o dólar acima de 10%
   num dia; a média nacional de um combustível acima de 15% numa semana), um
   mês da ANP com bem menos preços que o normal, uma semana com UFs faltando.
   Os limites foram calibrados com os dados reais, para não avisar sobre o
   que é normal;
3. **api** (fica no ar): serve a mart em http://localhost:8080;
4. **web** (fica no ar): o dashboard em **http://localhost:3000**.

Para coletar de novo (o pipeline roda em seguida), coletar só uma fonte, ou
só reprocessar o raw:

```bash
docker compose up collector pipeline
docker compose run --rm --no-deps -e COLLECTOR_SOURCES=ibge collector
docker compose run --rm --no-deps pipeline
docker compose run --rm --no-deps -e PIPELINE_JOBS=anp pipeline
```

Para buscar o histórico completo do BCB de novo, ou reconstruir staging e
mart do zero a partir de todo o raw (depois de mudar uma regra de
transformação, por exemplo):

```bash
docker compose run --rm --no-deps -e COLLECTOR_SOURCES=bcb_sgs -e SGS_FULL_HISTORY=true collector
docker compose run --rm --no-deps -e PIPELINE_FULL_REFRESH=true pipeline
```

Para ver os dados prontos:

```bash
docker compose exec postgres psql -U radar -c "SELECT * FROM mart.indicators"
docker compose exec postgres psql -U radar -c "SELECT * FROM mart.fuel_prices_weekly WHERE scope = 'BR' ORDER BY week_start DESC LIMIT 6"
docker compose exec postgres psql -U radar -c "SELECT job, status, finished_at, rows_written FROM mart.pipeline_runs ORDER BY id DESC LIMIT 6"
```

Cada componente:

```bash
go test ./collector/... ./api/...      # Go (a partir da raiz, via go.work)
# com testes de integração (precisa do Postgres do compose rodando):
TEST_DATABASE_URL=postgres://radar:radar@localhost:5432/radar?sslmode=disable go test ./collector/... ./api/...
DATABASE_URL=postgres://radar:radar@localhost:5432/radar?sslmode=disable go run ./api   # API fora do Docker
uv run --directory pipeline pytest      # pipeline (integração: mesma TEST_DATABASE_URL)
npm --prefix web install                # web: dependências
npm --prefix web test                   # web: testes (node --test, sem bibliotecas extras)
npm --prefix web run dev                # web fora do Docker, em http://localhost:3000 (usa a API em :8080)
```

> **No PowerShell**, a sintaxe `VAR=valor comando` não existe. Defina a variável
> antes, e ela vale para os próximos comandos no mesmo terminal:
> `$env:TEST_DATABASE_URL="postgres://radar:radar@localhost:5432/radar?sslmode=disable"`

## Dashboard

Next.js 16 (App Router, Cache Components) em http://localhost:3000, com duas
páginas, tema claro e escuro, e funcionando no celular.

**Indicadores** (`/`):

- **Cards** com o último valor de cada indicador: a última decisão do Copom
  sobre a Selic, o IPCA acumulado em 12 meses e a variação diária do dólar;
- **Gráficos** (ECharts): Selic em degraus (só muda nas reuniões do Copom),
  IPCA em barras (cada mês é uma variação, acima ou abaixo de zero) e dólar
  em linha, com um seletor de período único para os três;
- **Tabela** com os valores mais recentes ao lado de cada gráfico, como
  alternativa acessível.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/dashboard-escuro.png">
  <img alt="Página de indicadores: cards com a meta Selic (13,75% ao ano), o IPCA (0,82% em setembro, 4,58% em 12 meses) e o dólar (R$ 5,5013), seguidos de um gráfico de cada indicador nos últimos 5 anos" src="docs/img/dashboard.png">
</picture>

**Combustíveis** (`/combustiveis`):

- **Filtros** de combustível, estado e mês, guardados na URL
  (`/combustiveis?combustivel=etanol&uf=SP&mes=2026-10`): a página filtrada
  pode ser compartilhada, e o botão Voltar desfaz a última troca;
- **Cards** com o preço médio da última semana (e a variação em 4 semanas) e
  os municípios mais barato e mais caro do mês;
- **Gráfico semanal**: com um estado escolhido, a linha dele em destaque e a
  do Brasil em cinza, como referência. A linha é interrompida nas semanas sem
  pesquisa completa, em vez de ligar os pontos por cima do buraco;
- **Mapa** ([MapLibre](https://maplibre.org)) com um ponto por município
  pesquisado, colorido em cinco faixas de preço (quintis, com a mesma escala
  para o país inteiro); o estado escolhido fica em destaque e enquadrado;
- **Rankings** dos mais baratos e mais caros (só municípios com 3 ou mais
  postos) e a **tabela** de todos os municípios do mês.

O mapa não depende de nenhum serviço de mapas: o fundo é o contorno dos
estados, gerado a partir da malha do IBGE por `web/scripts/build-ufs-geojson.mjs`
e guardado em `web/public/geo/ufs.json`. O worker do MapLibre é copiado de
`node_modules` para `web/public/vendor/` antes do `dev` e do `build`.

Os dados são buscados no **servidor** do Next: o navegador nunca chama a API
(sem CORS, URL da API em `API_URL`, lida em tempo de execução). Trocar um
filtro é uma navegação: o servidor busca os dados novos, e enquanto isso a
página mostra a escolha nova e esmaece o conteúdo antigo. A parte que depende
da API fica dentro de um `<Suspense>`, então o `next build` gera o restante da
página como HTML estático e **não precisa da API no ar**.

## API

Somente leitura, sobre o schema `mart`. A especificação completa (OpenAPI 3.1)
fica em [`api/openapi.yaml`](api/openapi.yaml) e é servida em `/openapi.yaml`.

| Rota | O que devolve |
|---|---|
| `GET /health` | `{"status":"ok"}` |
| `GET /v1/indicators` | Indicadores (`selic`, `ipca`, `usd_brl`) com o valor mais recente |
| `GET /v1/indicators/{id}/values?from=AAAA-MM-DD&to=AAAA-MM-DD` | Série histórica; `from` e `to` são opcionais |
| `GET /v1/fuels` | Combustíveis (`gasolina`, `etanol`, `diesel_s10`...) com a média nacional da última semana |
| `GET /v1/fuels/{id}/prices?scope=BR\|UF&from=&to=` | Média semanal no Brasil (padrão) ou numa UF |
| `GET /v1/fuels/{id}/municipalities?month=AAAA-MM` | Preço em cada município pesquisado, com coordenadas (mapa); sem `month`, o mais recente |

```bash
curl "http://localhost:8080/v1/indicators/usd_brl/values?from=2026-11-01"
curl "http://localhost:8080/v1/fuels/gasolina/prices?scope=SP&from=2026-09-01"
```

Erros seguem sempre o formato `{"error": {"code": "...", "message": "..."}}`.
A conexão com o banco é aberta em modo somente leitura
(`default_transaction_read_only`): o Postgres recusa qualquer escrita da API.

As respostas do banco ficam guardadas em memória por `CACHE_TTL` (padrão
30 min; 1 min no Compose; `0` desliga): a mart muda no máximo uma vez por dia,
e assim o banco não é consultado a cada visita. Séries com período são
guardadas inteiras e recortadas em memória, então pedidos de períodos
diferentes da mesma série saem da mesma entrada (`api/internal/cachedstore`).

As queries ficam em `api/queries/` e o código Go correspondente é gerado pelo
[sqlc](https://sqlc.dev). Depois de mudar uma query ou o schema (rodar da raiz):

```bash
docker run --rm -v "${PWD}:/src" -w /src/api sqlc/sqlc:1.31.1 generate
```

## Deploy

Tudo em São Paulo, para cada visita fazer o caminho Vercel → API → banco
sem sair do Brasil.

| Parte | Onde | Como publica |
|---|---|---|
| Banco | [Neon](https://neon.com) (Postgres 16, AWS `sa-east-1`) | as migrations rodam nos workflows |
| Dados | GitHub Actions ([`data.yml`](.github/workflows/data.yml)) | disparo manual: migrations → coletor → pipeline |
| API | [Fly.io](https://fly.io) ([`api/fly.toml`](api/fly.toml), região `gru`) | `fly deploy --config api/fly.toml api` |
| Dashboard | [Vercel](https://vercel.com) (pasta `web`, região `gru1`) | a cada push na `main` |

Segredos nunca entram no código: `DATABASE_URL` (connection string **direta**
do Neon, sem `-pooler`, com `sslmode=require`) fica nos secrets do GitHub e do
Fly.io; `API_URL` (endereço da API no Fly.io) fica nas variáveis da Vercel.

## Migrations

Arquivos SQL numerados em `db/migrations/` (`NNNNNN_nome.up.sql` / `.down.sql`),
aplicados pelo serviço `migrate` do Compose. Para reverter tudo:

```bash
docker compose run --rm migrate -path=/migrations -database="postgres://radar:radar@postgres:5432/radar?sslmode=disable" down -all
```
