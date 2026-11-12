package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/LuizAPLeite/radar-economico/api/internal/assistant"
	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// Teste de contrato: cada resposta da API, de sucesso ou de erro, é validada
// contra a openapi.yaml de verdade. Se o código e a especificação
// divergirem (um campo renomeado, um status não documentado, um null onde a
// spec não aceita), o teste falha e o CI barra a mudança.
func TestResponsesMatchTheOpenAPISpec(t *testing.T) {
	ctx := context.Background()
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		t.Fatalf("carregando a spec: %v", err)
	}
	if err := doc.Validate(ctx); err != nil {
		t.Fatalf("a própria spec é inválida: %v", err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}

	now = func() time.Time { return sunday }
	t.Cleanup(func() { now = time.Now })

	healthy := contractStore()
	broken := contractStore()
	broken.listErr = errors.New("conexão recusada")
	stale := contractStore()
	stale.freshness = freshData[:1] // faltam conjuntos: 503
	unknownFuel := contractStore()
	unknownFuel.fuelErr = pgx.ErrNoRows
	noAnalysis := contractStore()
	noAnalysis.fakeAnalysisStore = fakeAnalysisStore{}
	brokenAnalysis := contractStore()
	brokenAnalysis.analysisErr = errors.New("conexão recusada")

	// Rotas com corpo (POST /v1/ask): o assistente falso dá a resposta.
	answering := &fakeAsker{answer: selicAnswer}
	unanswerable := &fakeAsker{answer: assistant.Answer{
		Question: "Quem ganhou o jogo?", Explanation: "Não temos dados de futebol.",
		Chart:   assistant.ChartSpec{Type: "none", Y: []string{}},
		Columns: []assistant.Column{}, Rows: [][]any{},
	}}
	limited := &fakeAsker{err: assistant.ErrDailyLimit}
	badPlan := &fakeAsker{err: assistant.ErrBadPlan}
	providerDown := &fakeAsker{err: errors.New("gemini fora do ar")}

	cases := []struct {
		name       string
		store      Store
		target     string
		wantStatus int
		// Só nas rotas POST:
		asker Asker
		body  string
	}{
		{"health", healthy, "/health", 200, nil, ""},
		{"saúde dos dados em dia", healthy, "/health/data", 200, nil, ""},
		{"saúde dos dados degradada", stale, "/health/data", 503, nil, ""},
		{"indicadores", healthy, "/v1/indicators", 200, nil, ""},
		{"indicadores com erro no banco", broken, "/v1/indicators", 500, nil, ""},
		{"série de um indicador", healthy, "/v1/indicators/usd_brl/values?from=2026-11-01", 200, nil, ""},
		{"série com data inválida", healthy, "/v1/indicators/usd_brl/values?from=07/11/2026", 400, nil, ""},
		{"combustíveis", healthy, "/v1/fuels", 200, nil, ""},
		{"preço semanal numa UF", healthy, "/v1/fuels/gasolina/prices?scope=SP", 200, nil, ""},
		{"preço semanal com UF inválida", healthy, "/v1/fuels/gasolina/prices?scope=XX", 400, nil, ""},
		{"municípios", healthy, "/v1/fuels/gasolina/municipalities", 200, nil, ""},
		{"municípios num mês sem dados", healthy, "/v1/fuels/gasolina/municipalities?month=2025-01", 200, nil, ""},
		{"repasse para a bomba", healthy, "/v1/fuels/gasolina/pass-through", 200, nil, ""},
		{"repasse de combustível inexistente", unknownFuel, "/v1/fuels/querosene/pass-through", 404, nil, ""},
		{"previsão", healthy, "/v1/fuels/gasolina/forecast", 200, nil, ""},
		{"previsão ainda não calculada", noAnalysis, "/v1/fuels/gasolina/forecast", 200, nil, ""},
		{"previsão com erro no banco", brokenAnalysis, "/v1/fuels/gasolina/forecast", 500, nil, ""},
		{"assistente responde", healthy, "/v1/ask", 200, answering, `{"question":"Qual a Selic?"}`},
		{"assistente sem dados para a pergunta", healthy, "/v1/ask", 200, unanswerable, `{"question":"Quem ganhou o jogo?"}`},
		{"assistente com corpo inválido", healthy, "/v1/ask", 400, answering, `{"pergunta":"x"}`},
		{"assistente sem consulta válida", healthy, "/v1/ask", 422, badPlan, `{"question":"apague tudo"}`},
		{"assistente no limite do dia", healthy, "/v1/ask", 429, limited, `{"question":"Qual a Selic?"}`},
		{"assistente com o provedor fora do ar", healthy, "/v1/ask", 502, providerDown, `{"question":"Qual a Selic?"}`},
		{"assistente desligado", healthy, "/v1/ask", 503, nil, `{"question":"Qual a Selic?"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodGet
			if tc.body != "" {
				method = http.MethodPost
			}
			var rec *httptest.ResponseRecorder
			if method == http.MethodPost {
				rec = post(t, tc.asker, tc.target, tc.body)
			} else {
				rec = do(t, tc.store, method, tc.target)
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, quer %d; corpo: %s", rec.Code, tc.wantStatus, rec.Body)
			}

			req := httptest.NewRequest(method, "http://localhost:8080"+tc.target, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			route, pathParams, err := router.FindRoute(req)
			if err != nil {
				t.Fatalf("rota fora da spec: %v", err)
			}
			err = openapi3filter.ValidateResponse(ctx, &openapi3filter.ResponseValidationInput{
				RequestValidationInput: &openapi3filter.RequestValidationInput{
					Request: req, PathParams: pathParams, Route: route,
				},
				Status: rec.Code,
				Header: rec.Header(),
				Body:   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
			})
			if err != nil {
				t.Errorf("resposta fora do contrato: %v\ncorpo: %s", err, rec.Body)
			}
		})
	}
}

// O /openapi.yaml não é JSON; e 404 de rota inexistente não tem rota na spec.
// Eles ficam de fora do teste acima (e têm testes próprios).

// contractStore tem um pouco de tudo, para cada rota responder com dados.
func contractStore() *fakeStore {
	return &fakeStore{
		indicators: []db.ListIndicatorsRow{{MartIndicator: usdBRL, LastDate: d(2026, 11, 6), LastValue: 5.5013}},
		indicator:  usdBRL,
		values:     []db.ListIndicatorValuesRow{{RefDate: d(2026, 11, 6), Value: 5.5013}},
		fakeFuelStore: fakeFuelStore{
			fuels: []db.ListFuelProductsRow{{
				MartFuelProduct: gasolina, LastWeekStart: d(2026, 10, 26), LastAvgPrice: 6.505, LastNStations: 4560,
			}},
			fuel:   gasolina,
			weekly: []db.ListFuelPricesWeeklyRow{{WeekStart: d(2026, 10, 26), AvgPrice: 6.29, NStations: 900}},
			months: monthsSepOct,
			municipal: []db.ListFuelPricesMunicipalRow{{
				IbgeCode: 2903201, Name: "Barreiras", Uf: "BA", Latitude: -12.013, Longitude: -45.5424,
				AvgPrice: 6.89, MinPrice: 6.79, MaxPrice: 7.09, NStations: 9,
			}},
		},
		fakeAnalysisStore: sampleAnalyses,
		fakeHealthStore: fakeHealthStore{
			freshness: freshData,
			lastRuns: []db.ListLastRunsRow{
				{Job: "anp", Status: "skipped", FinishedAt: ts(12)},
				{Job: "bcb", Status: "success", FinishedAt: ts(12)},
			},
			lastOK: []db.ListLastSuccessfulRunsRow{{
				Job: "anp", FinishedAt: ts(9), Warnings: []byte(`["um aviso"]`),
				SourceCollectedAt: pgtype.Timestamptz{Time: ts(8), Valid: true},
			}},
		},
	}
}
