# Radar Econômico

Coleta dados públicos brasileiros (Banco Central, IBGE e ANP), processa em
camadas e exibe num dashboard web com gráficos e mapa.

> Em construção — Fase 0 (fundação do monorepo).

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
docker compose up -d          # Postgres, Redis e aplica as migrations
docker compose exec postgres psql -U radar -c '\dn'   # deve listar raw, staging, mart
```

O `docker compose up` também roda, uma vez cada, em ordem:

1. **collector**: busca as séries do Banco Central e grava as respostas brutas
   em `raw.bcb_sgs_responses`;
2. **pipeline**: lê a coleta mais recente, valida e grava
   `staging.bcb_sgs_observations` e `mart.indicator_values`.

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
TEST_DATABASE_URL=postgres://radar:radar@localhost:5432/radar?sslmode=disable go test ./collector/...
go run ./api                            # API em http://localhost:8080/health
cd pipeline && uv run pytest            # pipeline (integração: mesma TEST_DATABASE_URL)
cd web && npm install && npm run dev    # front em http://localhost:3000
```

## Migrations

Arquivos SQL numerados em `db/migrations/` (`NNNNNN_nome.up.sql` / `.down.sql`),
aplicados pelo serviço `migrate` do Compose. Para reverter tudo:

```bash
docker compose run --rm migrate -path=/migrations -database="postgres://radar:radar@postgres:5432/radar?sslmode=disable" down -all
```
