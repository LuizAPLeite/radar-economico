# Radar Econômico

Coleta dados públicos brasileiros (Banco Central, IBGE e ANP), processa em
camadas e exibe num dashboard web com gráficos e mapa.

> Em construção — Fase 1 (indicadores do Banco Central de ponta a ponta).

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
| `web/` | Next.js, TypeScript, Tailwind | Dashboard |
| `db/migrations/` | SQL (golang-migrate) | Schemas e tabelas |

## Rodando localmente

Pré-requisitos: Docker, Go, Python 3.13 + [uv](https://docs.astral.sh/uv/), Node.js.

```bash
cp .env.example .env
docker compose up -d          # banco, migrations, coleta, pipeline, API e dashboard
docker compose exec postgres psql -U radar -c '\dn'   # deve listar raw, staging, mart
```

Além de Postgres e Redis, o `docker compose up` sobe:

1. **collector** (roda uma vez): busca as séries do Banco Central e grava as
   respostas brutas em `raw.bcb_sgs_responses`;
2. **pipeline** (roda uma vez, depois do collector): lê a coleta mais recente,
   valida e grava `staging.bcb_sgs_observations` e `mart.indicator_values`;
3. **api** (fica no ar): serve a mart em http://localhost:8080;
4. **web** (fica no ar): o dashboard em **http://localhost:3000**.

Para coletar de novo (o pipeline roda em seguida), ou só reprocessar o raw:

```bash
docker compose up collector pipeline
docker compose run --rm --no-deps pipeline
```

Para ver os dados prontos:

```bash
docker compose exec postgres psql -U radar -c "SELECT * FROM mart.indicators"
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

Next.js 16 (App Router, Cache Components) em http://localhost:3000:

- **Cards** com o último valor de cada indicador: a última decisão do Copom
  sobre a Selic, o IPCA acumulado em 12 meses e a variação diária do dólar;
- **Gráficos** (ECharts): Selic em degraus (só muda nas reuniões do Copom),
  IPCA em barras (cada mês é uma variação, acima ou abaixo de zero) e dólar
  em linha, com um seletor de período único para os três;
- **Tabela** com os valores mais recentes ao lado de cada gráfico, como
  alternativa acessível; tema claro e escuro; funciona no celular.

Os dados são buscados no **servidor** do Next: o navegador nunca chama a API
(sem CORS, URL da API em `API_URL`, lida em tempo de execução). A parte que
depende da API fica dentro de um `<Suspense>`, então o `next build` gera o
restante da página como HTML estático e **não precisa da API no ar**.

## API

Somente leitura, sobre o schema `mart`. A especificação completa (OpenAPI 3.1)
fica em [`api/openapi.yaml`](api/openapi.yaml) e é servida em `/openapi.yaml`.

| Rota | O que devolve |
|---|---|
| `GET /health` | `{"status":"ok"}` |
| `GET /v1/indicators` | Indicadores (`selic`, `ipca`, `usd_brl`) com o valor mais recente |
| `GET /v1/indicators/{id}/values?from=AAAA-MM-DD&to=AAAA-MM-DD` | Série histórica; `from` e `to` são opcionais |

```bash
curl "http://localhost:8080/v1/indicators/usd_brl/values?from=2026-11-01"
```

Erros seguem sempre o formato `{"error": {"code": "...", "message": "..."}}`.
A conexão com o banco é aberta em modo somente leitura
(`default_transaction_read_only`): o Postgres recusa qualquer escrita da API.

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
