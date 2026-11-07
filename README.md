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
docker compose up -d          # banco, migrations, coleta, pipeline e API
docker compose exec postgres psql -U radar -c '\dn'   # deve listar raw, staging, mart
```

Além de Postgres e Redis, o `docker compose up` sobe:

1. **collector** (roda uma vez): busca as séries do Banco Central e grava as
   respostas brutas em `raw.bcb_sgs_responses`;
2. **pipeline** (roda uma vez, depois do collector): lê a coleta mais recente,
   valida e grava `staging.bcb_sgs_observations` e `mart.indicator_values`;
3. **api** (fica no ar): serve a mart em http://localhost:8080.

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
cd web && npm install && npm run dev    # front em http://localhost:3000
```

> **No PowerShell**, a sintaxe `VAR=valor comando` não existe. Defina a variável
> antes, e ela vale para os próximos comandos no mesmo terminal:
> `$env:TEST_DATABASE_URL="postgres://radar:radar@localhost:5432/radar?sslmode=disable"`

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

## Migrations

Arquivos SQL numerados em `db/migrations/` (`NNNNNN_nome.up.sql` / `.down.sql`),
aplicados pelo serviço `migrate` do Compose. Para reverter tudo:

```bash
docker compose run --rm migrate -path=/migrations -database="postgres://radar:radar@postgres:5432/radar?sslmode=disable" down -all
```
