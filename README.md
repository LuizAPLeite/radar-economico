# Radar Econômico

**Dados públicos da economia brasileira, de ponta a ponta:** coleta de cinco
fontes, pipeline em camadas com validação, API REST, dashboard com gráficos e
mapa, análises estatísticas e um assistente que responde perguntas em
português escrevendo o SQL. Tudo em produção, com custo zero.

**[Ver o site no ar](https://radar-economico-phi.vercel.app)** ·
[API](https://radar-economico-api.onrender.com/health/data) ·
[Especificação OpenAPI](https://radar-economico-api.onrender.com/openapi.yaml)

![Demonstração: a página de indicadores, o mapa de preços da gasolina por município, as análises de repasse do petróleo para a bomba e o assistente respondendo "Qual estado tem o diesel S10 mais caro?" com Acre, R$ 8,305](docs/img/demo.gif)

## O que dá para ver

| | |
|---|---|
| <picture><source media="(prefers-color-scheme: dark)" srcset="docs/img/inicio-escuro.png"><img alt="Indicadores: cards da Selic (13,75%), do IPCA (0,82% em setembro), do dólar (R$ 5,56) e do Brent (US$ 107,03), e o gráfico da Selic em degraus nos últimos 5 anos" src="docs/img/inicio.png"></picture> **Indicadores**: Selic, IPCA, dólar e petróleo Brent, com um seletor de período para todos os gráficos. | <picture><source media="(prefers-color-scheme: dark)" srcset="docs/img/combustiveis-escuro.png"><img alt="Combustíveis: gasolina a R$ 6,51 no Brasil, o município mais barato (Goiatuba, R$ 5,74) e o mais caro (Parintins, R$ 8,99), o gráfico semanal e o mapa com um ponto por município" src="docs/img/combustiveis.png"></picture> **Combustíveis**: preço semanal por estado e o mapa dos ~400 municípios pesquisados pela ANP. |
| <picture><source media="(prefers-color-scheme: dark)" srcset="docs/img/analises-escuro.png"><img alt="Análises do diesel S10: desde 2023, a bomba acompanha o petróleo em reais uma semana depois (correlação de 0,55); o dólar sozinho explica pouco (0,15). Gráfico de barras por período da política de preços" src="docs/img/analises.png"></picture> **Análises**: quanto a bomba acompanha o petróleo e o dólar desde 2004, e a previsão testada contra o passado. | <picture><source media="(prefers-color-scheme: dark)" srcset="docs/img/assistente-escuro.png"><img alt="Assistente: a pergunta 'Como o preço médio da gasolina variou por ano desde 2015?' respondida com um gráfico de linha de R$ 3,35 em 2015 a R$ 6,54 em 2026 e a tabela por ano" src="docs/img/assistente.png"></picture> **Assistente**: pergunta em português → SQL escrito pelo Gemini → gráfico, tabela e o SQL usado. |

## Destaques técnicos

**Engenharia de dados**
- Arquitetura em camadas **raw → staging → mart** no PostgreSQL: o dado bruto
  fica intacto (JSON e arquivos originais, compactados), e qualquer regra pode
  ser reprocessada a partir dele.
- **Cargas incrementais** em todas as fontes (o BCB busca só as últimas
  semanas; a ANP, só os meses novos; planilhas e CSVs só entram quando o
  SHA-256 muda) e **upsert** que nunca apaga histórico.
- **Validação em dois níveis**: contrato com pandera (falhou, a mart fica
  intacta) e avisos de qualidade calibrados com os dados reais (que não barram
  a carga). Cada execução fica registrada, com origem, linhas e avisos.
- Conferência cruzada: na gasolina, a média que o pipeline calcula dos ~780
  mil preços por posto fica a 0,4% da média oficial da ANP.

**Back-end (Go)**
- Coletor concorrente (goroutines, `errgroup` com limite, retry com backoff e
  jitter, logs estruturados) e API com chi, pgx e **sqlc** (SQL tipado).
- **Teste de contrato**: cada resposta da API, de sucesso e de erro, é validada
  contra a `openapi.yaml`; a spec não tem como ficar desatualizada.
- Cache em memória genérico (decorator da camada de dados) para o banco
  gratuito dormir entre as visitas.

**Análises e honestidade estatística**
- Correlação com defasagem por período da política de preços da Petrobras: a
  bomba segue o **petróleo em reais** com 1 a 3 semanas de atraso desde 2016;
  o dólar sozinho quase não explica.
- Previsão com **backtest** de 104 semanas: o modelo simples ("o preço fica
  igual") venceu a regressão com o petróleo, e a página publica o vencedor e
  diz por quê.

**LLM com segurança**
- O Gemini escreve o SQL; a API aplica **quatro camadas**: prompt restrito,
  validação do SQL, um papel do Postgres que só lê tabelas públicas e
  transação somente leitura com tempo máximo. Cota grátis protegida por cache
  e limite diário; o provedor fica atrás de uma interface.

**Front-end e produto**
- Next.js 16 com Server Components (o navegador nunca fala com a API), gráficos
  ECharts e mapa MapLibre sem serviço de mapas, modo claro e escuro, celular,
  e uma tabela acessível ao lado de cada gráfico.

**Operação**
- CI com testes de Go, Python e web, migrations up → down → up e `sqlc diff`;
  coleta diária agendada; **canário** diário que refaz o caminho inteiro
  contra as fontes reais e avisa quando uma fonte muda de formato.
- Em produção com **custo zero**: Vercel, Render, Neon, GitHub Actions,
  UptimeRobot e o plano grátis do Gemini.

## Arquitetura

```mermaid
flowchart LR
    subgraph fontes["Fontes públicas"]
        BCB["Banco Central<br/>Selic, IPCA, dólar"]
        ANP["ANP<br/>preços por posto e<br/>série semanal desde 2004"]
        IBGE["IBGE<br/>municípios"]
        FRED["FRED<br/>petróleo Brent"]
    end

    subgraph pg["PostgreSQL (Neon)"]
        RAW[("raw<br/>dado bruto intacto")]
        STG[("staging<br/>limpo e tipado")]
        MART[("mart<br/>pronto para servir")]
    end

    COL["collector<br/>Go"]
    PIPE["pipeline<br/>Python, polars, pandera"]
    API["api<br/>Go, chi, sqlc"]
    LLM["Gemini<br/>escreve o SQL"]
    WEB["web<br/>Next.js, ECharts, MapLibre"]

    BCB & ANP & IBGE & FRED --> COL --> RAW
    RAW --> PIPE
    PIPE --> STG
    PIPE --> MART
    MART -->|"somente leitura"| API
    API <-->|"pergunta → SQL"| LLM
    API --> WEB

    GHA["GitHub Actions<br/>CI, coleta diária, canário"] -.-> COL
    GHA -.-> PIPE
```

O coletor e o pipeline não se chamam: comunicam-se pelo banco. A API só lê o
schema `mart`, numa conexão que o Postgres mantém somente leitura.

| Pasta | Stack | Papel |
|---|---|---|
| `collector/` | Go | Coleta concorrente das fontes, grava em `raw` |
| `pipeline/` | Python, polars, pandera | Limpa e valida `raw` → `staging` → `mart`; análises |
| `api/` | Go, chi, pgx, sqlc | API REST somente leitura sobre `mart` e o assistente (LLM → SQL) |
| `web/` | Next.js, TypeScript, Tailwind, ECharts, MapLibre | Dashboard com gráficos, mapa e assistente |
| `db/migrations/` | SQL (golang-migrate) | Schemas, tabelas e o papel restrito do assistente |

| Onde roda | Serviço | Plano |
|---|---|---|
| Site | Vercel (São Paulo) | Hobby, grátis |
| API | Render (Virginia), mantida acordada pelo UptimeRobot | Free |
| Banco | Neon, Postgres 16 (São Paulo) | Free |
| Coleta, CI e canário | GitHub Actions | grátis |
| Assistente | Gemini 3.5 Flash-Lite | plano grátis |

## Rodando localmente

Pré-requisitos: Docker, Go, Python 3.13 + [uv](https://docs.astral.sh/uv/), Node.js.

```bash
cp .env.example .env
docker compose up -d          # banco, migrations, coleta, pipeline, API e dashboard
docker compose exec postgres psql -U radar -c '\dn'   # deve listar raw, staging, mart
```

Além do Postgres, o `docker compose up` sobe:

1. **collector** (roda uma vez): coleta em paralelo o Banco Central (Selic,
   IPCA, dólar), o IBGE (municípios e coordenadas), a ANP (preços de
   combustíveis por posto, últimos 12 meses, e a série histórica semanal do
   Brasil desde 2004, em planilhas) e o FRED (petróleo Brent diário) e grava
   tudo, intacto, no schema `raw`. Planilhas e CSVs só são gravados de novo
   quando o conteúdo muda (SHA-256). As coletas são **incrementais**: a primeira traz o histórico (o BCB
   desde 2000; a primeira da ANP leva alguns minutos, porque o servidor é
   lento), e as seguintes só buscam o que pode ter mudado. No BCB, isso são
   as últimas semanas de cada série, com uma margem para valores publicados
   com atraso ou corrigidos: ~3 KB por coleta, em vez de ~100 KB;
2. **pipeline** (roda uma vez, depois do collector): seis jobs, cada um lendo
   só o que chegou desde a última execução, validando com pandera e
   gravando `staging` e `mart`:
   - **bcb**: indicadores (`mart.indicator_values`). Junta as coletas novas e
     grava com upsert (insere as datas novas, atualiza os valores corrigidos
     e não apaga nada);
   - **ibge**: municípios com coordenadas (`mart.municipalities`);
   - **anp**: ~780 mil preços por posto (`staging.fuel_prices`), casados com os
     municípios do IBGE, e as médias semanais por UF e Brasil
     (`mart.fuel_prices_weekly`) e mensais por município
     (`mart.fuel_prices_municipal`). Reconstrói tudo em uns 5 segundos quando
     chega arquivo novo;
   - **fred**: o Brent como indicador (`mart.indicator_values`, ao lado dos do BCB);
   - **anp_serie**: a média semanal oficial da ANP desde 2004
     (`mart.fuel_prices_anp_weekly`). Um aviso compara com a média que o job
     **anp** calcula dos postos: na gasolina e no diesel elas diferem 0,4% a
     1,1%; no etanol e no GNV a oficial é ponderada e a diferença é estrutural;
   - **analysis**: as correlações e a previsão (ver [Análises](#análises)),
     a partir do que os outros jobs gravaram na mart.

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

Next.js 16 (App Router, Cache Components) em http://localhost:3000, com quatro
páginas, tema claro e escuro, e funcionando no celular.

**Indicadores** (`/`):

- **Cards** com o último valor de cada indicador: a última decisão do Copom
  sobre a Selic, o IPCA acumulado em 12 meses e a variação diária do dólar e
  do petróleo Brent;
- **Gráficos** (ECharts): Selic em degraus (só muda nas reuniões do Copom),
  IPCA em barras (cada mês é uma variação, acima ou abaixo de zero) e dólar
  em linha, com um seletor de período único para os três;
- **Tabela** com os valores mais recentes ao lado de cada gráfico, como
  alternativa acessível.

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

**Análises** (`/analises`), por combustível (gasolina, diesel S10 e S500,
etanol): a correlação com o petróleo em reais e com o dólar em cada período
da política de preços, por semanas de defasagem e em janela móvel, e a
previsão das próximas 8 semanas com a faixa de 80% e o erro de cada modelo
no teste com o passado. Cada gráfico tem legenda, rótulos diretos e tabela.

**Assistente** (`/assistente`): uma caixa de pergunta em português, com
cinco perguntas de exemplo; a resposta vem com título, explicação, gráfico
(linha ou barras), tabela e o SQL usado. Ver [Assistente](#assistente).

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

Somente leitura sobre o schema `mart` (o `POST /v1/ask` recebe a pergunta no
corpo, mas também não grava nada). A especificação completa (OpenAPI 3.1)
fica em [`api/openapi.yaml`](api/openapi.yaml) e é servida em `/openapi.yaml`.

| Rota | O que devolve |
|---|---|
| `GET /health` | `{"status":"ok"}` |
| `GET /health/data` | Se os dados estão em dia: a data mais recente de cada conjunto, a última execução de cada job e os avisos de qualidade. 200 ou 503 |
| `GET /v1/indicators` | Indicadores (`selic`, `ipca`, `usd_brl`, `brent`) com o valor mais recente |
| `GET /v1/indicators/{id}/values?from=AAAA-MM-DD&to=AAAA-MM-DD` | Série histórica; `from` e `to` são opcionais |
| `GET /v1/fuels` | Combustíveis (`gasolina`, `etanol`, `diesel_s10`...) com a média nacional da última semana |
| `GET /v1/fuels/{id}/prices?scope=BR\|UF&from=&to=` | Média semanal no Brasil (padrão) ou numa UF |
| `GET /v1/fuels/{id}/municipalities?month=AAAA-MM` | Preço em cada município pesquisado, com coordenadas (mapa); sem `month`, o mais recente |
| `POST /v1/ask` | Corpo `{"question": "..."}`: a resposta do assistente (dados, SQL e sugestão de gráfico). Ver [Assistente](#assistente) |
| `GET /v1/fuels/{id}/pass-through` | Correlação do preço com o dólar e o Brent, por período da política de preços e defasagem, e a correlação móvel |
| `GET /v1/fuels/{id}/forecast` | Previsão de 1 a 8 semanas com intervalo de 80%, o erro de cada modelo no backtest e as últimas 104 semanas da série oficial |

```bash
curl "http://localhost:8080/v1/indicators/usd_brl/values?from=2026-11-01"
curl "http://localhost:8080/v1/fuels/gasolina/prices?scope=SP&from=2026-09-01"
```

Erros seguem sempre o formato `{"error": {"code": "...", "message": "..."}}`.
A especificação não fica desatualizada: um teste de contrato
([kin-openapi](https://github.com/getkin/kin-openapi)) valida cada resposta
das rotas, de sucesso e de erro, contra a `openapi.yaml`, e o CI barra
qualquer divergência.
A conexão com o banco é aberta em modo somente leitura
(`default_transaction_read_only`): o Postgres recusa qualquer escrita da API.

O `GET /health/data` compara a data do dado mais recente de cada conjunto com
o ritmo da fonte: até 5 dias para a Selic e o dólar (fins de semana e
feriados), 75 para o IPCA (que sai por volta do dia 10 do mês seguinte), 60
para a ANP (arquivos mensais, publicados semanas depois), 15 para o Brent (o
FRED publica com cerca de uma semana de atraso) e 21 para a série semanal
oficial da ANP. Responde 503 quando
algo atrasou ou quando a última execução de um job falhou, para um monitor
externo avisar; a mensagem de erro do job fica só no banco.

```bash
curl -s http://localhost:8080/health/data
```

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

## Análises

Calculadas pelo job **analysis** do pipeline (`pipeline/src/radar_pipeline/analysis/`),
com a série semanal oficial da ANP (desde 2004), o dólar do BCB e o Brent do
FRED. O petróleo em reais é o Brent multiplicado pelo dólar do mesmo dia.

**Repasse para a bomba.** Correlação entre a variação em 4 semanas do preço
médio no Brasil e a variação do fator 0 a 12 semanas antes (variações, e não
níveis: os níveis sobem juntos com a inflação e dariam 0,92 de correlação sem
dizer nada). Calculada separadamente em cada período da política de preços da
Petrobras: preços administrados (até out/2016), paridade internacional (até
mai/2023) e a estratégia atual. O que os dados mostram:

| | 2004–2016 | 2016–2023 | Desde 2023 |
|---|---|---|---|
| Gasolina × petróleo em R$ | 0,06 | 0,49 (3 semanas depois) | 0,41 (2 semanas) |
| Diesel S10 × petróleo em R$ | 0,19 | 0,49 (2 semanas) | 0,55 (1 semana) |
| Gasolina × dólar | 0,13 | −0,07 | 0,14 |

O dólar sozinho quase não explica a bomba: quando o petróleo cai, o real
costuma se desvalorizar, e os dois efeitos se compensam (correlação de −0,36
entre as variações do dólar e do Brent).

**Previsão.** Dois modelos, testados nas últimas 104 semanas com o que se
sabia em cada semana (backtest): repetir o último preço (*naive*) e uma
regressão da variação futura sobre a variação recente do petróleo em reais. A
página publica o de menor erro, com o intervalo de 80% tirado dos erros do
teste. Nos dados reais, o *naive* vence em todos os combustíveis (a regressão
erra de 19% a 47% a mais): a bomba segue o petróleo, mas os reajustes vêm em
degraus e em datas que ninguém sabe antes. Uma variação com o "atraso" da
bomba em relação ao petróleo também foi testada, e errou mais.

## Assistente

Pergunta em português → um modelo de linguagem escreve o SQL → a API confere,
executa e devolve os dados (`api/internal/assistant`).

```
pergunta ─► Server Action (Next) ─► POST /v1/ask ─► Gemini: SQL + título + gráfico
                                        │
                                        └─► CheckSQL ─► Postgres (papel restrito) ─► dados
```

- **Modelo:** Gemini 3.5 Flash-Lite, no plano grátis (sem cartão): 500 perguntas
  por dia e 15 por minuto. O 3.8 Flash ficou sem responder no plano grátis
  durante os testes. O provedor fica atrás da interface `Planner`: trocar de
  modelo (ou de empresa) não muda o resto. A chamada é HTTP puro, com saída
  estruturada (o modelo devolve um JSON num esquema fixo) e `store: false`.
- **Uma chamada por pergunta:** o modelo devolve o SQL, o título, a explicação
  e o tipo de gráfico de uma vez; os números saem do banco, não do modelo.
- **Quatro camadas de proteção** para o SQL escrito pelo modelo:
  1. o prompt descreve só as tabelas públicas e manda ignorar instruções
     dentro da pergunta;
  2. `CheckSQL` aceita um único `SELECT`/`WITH`, sem comentários;
  3. a consulta roda no papel `radar_assistant` (migration 000008), que só lê
     tabelas da mart listadas uma a uma (sem `raw`, `staging` nem o registro de
     execuções), dentro de `SET LOCAL ROLE`;
  4. transação somente leitura, `statement_timeout` de 3 s e até 500 linhas.
- **Cota:** respostas guardadas por 24 h (perguntas iguais, ignorando maiúsculas
  e espaços, não gastam cota) e um limite de perguntas novas por dia
  (`ASSISTANT_DAILY_LIMIT`, padrão 400). Acabou a cota, a página explica e
  as perguntas de exemplo continuam respondendo do cache.
- **Gráfico conferido:** a sugestão do modelo só vale se as colunas existirem e
  os tipos baterem; uma série noutra escala fica só na tabela (um eixo só).

Para usar localmente, crie uma chave em [aistudio.google.com](https://aistudio.google.com)
(Get API key; sem ativar faturamento) e ponha no `.env` como `GEMINI_API_KEY`.
Sem a chave, o resto do site funciona e o assistente responde que está desligado.

```powershell
curl.exe -X POST http://localhost:8080/v1/ask -H "Content-Type: application/json" -d '{\"question\": \"Qual estado tem a gasolina mais cara hoje?\"}'
```

## Deploy

Tudo em planos gratuitos, sem cartão de crédito.

| Parte | Onde | Como publica |
|---|---|---|
| Banco | [Neon](https://neon.com) Free (Postgres 16, AWS `sa-east-1`, São Paulo) | as migrations rodam nos workflows |
| Dados | GitHub Actions ([`data.yml`](.github/workflows/data.yml)) | todo dia às 9h (com a variável `COLETA_AGENDADA=true`) ou disparo manual: migrations → coletor (BCB, ANP, série da ANP, Brent) → pipeline |
| API | [Render](https://render.com) Free ([`render.yaml`](render.yaml), Blueprint, região Virginia) | a cada push na `main` que mexa em `api/` |
| Dashboard | [Vercel](https://vercel.com) Hobby (pasta `web`, região `gru1`, São Paulo) | a cada push na `main` |

O plano grátis do Render desliga a API após 15 min sem requisições, e ela
leva ~1 min para religar. Um monitor do [UptimeRobot](https://uptimerobot.com)
(plano grátis) consulta `/health` a cada 5 min: mantém a API acordada (as
750 h grátis do mês cobrem o mês todo) e avisa por e-mail se ela cair. Como
`/health` não toca no banco, o Neon continua dormindo quando ninguém visita.
Se a API ainda assim estiver dormindo, o dashboard espera até 70 s por ela.

Um workflow agendado do GitHub Actions fez esse papel no começo, mas o
GitHub atrasa agendamentos curtos: em dois dias, o "a cada 10 min" rodou
7 vezes, com intervalos de até 8 horas.

Segredos nunca entram no código: `DATABASE_URL` (connection string **direta**
do Neon, sem `-pooler`, com `sslmode=require`) fica nos secrets do GitHub e
no painel do Render. `GEMINI_API_KEY` fica só no painel do Render (e no `.env`
local). `API_URL` (endereço da API no Render) fica nas variáveis da Vercel.

A coleta agendada não alcança o IBGE (ver o canário, abaixo). Os municípios
quase nunca mudam: carregue-os uma vez, e quando o IBGE criar um município
novo, a partir do seu computador. Defina a connection string do Neon na
variável `DATABASE_URL` do terminal (no PowerShell,
`$env:DATABASE_URL="..."`; nunca no código nem no git): o `-e DATABASE_URL`
sem valor repassa a variável do terminal para o container.

```bash
docker compose run --rm --no-deps -e DATABASE_URL -e COLLECTOR_SOURCES=ibge collector
docker compose run --rm --no-deps -e DATABASE_URL -e PIPELINE_JOBS=ibge pipeline
```

## Canário das fontes

Fontes públicas mudam sem aviso: uma coluna nova no CSV da ANP, um endpoint
diferente no Banco Central. O workflow [`fontes.yml`](.github/workflows/fontes.yml)
roda todo dia às 7h30 e refaz o caminho inteiro contra as fontes reais,
num Postgres temporário do próprio GitHub Actions: migrations → coleta
(BCB, a ANP só dos 2 últimos meses, a série histórica da ANP e o Brent) →
pipeline (inclusive as análises) → API, que precisa responder `/health/data`
com 200 e trazer a previsão da gasolina com as 8 semanas. Se algo mudou na fonte, ele falha e o GitHub avisa por
e-mail no mesmo dia, antes de a coleta de produção dar problema. Os erros do
coletor aparecem como anotações na página da execução, e o resultado de
`/health/data` fica no resumo.

Logo na primeira execução, o canário mostrou que **o IBGE recusa conexões
vindas de fora do Brasil**, e os servidores do Actions ficam nos EUA. Por
isso, nem o canário nem a coleta de produção buscam o IBGE. No
canário, os municípios (de que o job da ANP precisa) vêm de um retrato de
93 KB em [`.github/canario/`](.github/canario/); em produção, são carregados
uma vez a partir de uma máquina no Brasil (ver acima).

## Migrations

Arquivos SQL numerados em `db/migrations/` (`NNNNNN_nome.up.sql` / `.down.sql`),
aplicados pelo serviço `migrate` do Compose. Para reverter tudo:

```bash
docker compose run --rm migrate -path=/migrations -database="postgres://radar:radar@postgres:5432/radar?sslmode=disable" down -all
```

## Como o projeto evoluiu

Construído em fases, cada uma funcionando de ponta a ponta antes da seguinte:

1. **Banco Central** (Selic, IPCA, dólar): monorepo, Docker Compose,
   migrations, coletor, pipeline, API, dashboard e deploy.
2. **ANP**: ~780 mil preços por posto por ano, casados com os municípios do
   IBGE, filtros na URL e o mapa.
3. **Operação dos dados**: cargas incrementais, validação em dois níveis,
   registro das execuções, cache, saúde dos dados, testes de contrato, coleta
   agendada e o canário.
4. **Análises**: série oficial da ANP desde 2004, petróleo Brent, repasse
   por período da política de preços e previsão com backtest.
5. **Assistente**: pergunta em português → SQL pelo Gemini → resposta, com
   quatro camadas de proteção.
6. **Acabamento**: este README, o diagrama e a demonstração.
