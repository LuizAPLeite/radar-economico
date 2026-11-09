package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/jackc/pgx/v5/pgtype"

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

	cases := []struct {
		name       string
		store      Store
		target     string
		wantStatus int
	}{
		{"health", healthy, "/health", 200},
		{"saúde dos dados em dia", healthy, "/health/data", 200},
		{"saúde dos dados degradada", stale, "/health/data", 503},
		{"indicadores", healthy, "/v1/indicators", 200},
		{"indicadores com erro no banco", broken, "/v1/indicators", 500},
		{"série de um indicador", healthy, "/v1/indicators/usd_brl/values?from=2026-11-01", 200},
		{"série com data inválida", healthy, "/v1/indicators/usd_brl/values?from=07/11/2026", 400},
		{"combustíveis", healthy, "/v1/fuels", 200},
		{"preço semanal numa UF", healthy, "/v1/fuels/gasolina/prices?scope=SP", 200},
		{"preço semanal com UF inválida", healthy, "/v1/fuels/gasolina/prices?scope=XX", 400},
		{"municípios", healthy, "/v1/fuels/gasolina/municipalities", 200},
		{"municípios num mês sem dados", healthy, "/v1/fuels/gasolina/municipalities?month=2025-01", 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, tc.store, http.MethodGet, tc.target)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, quer %d; corpo: %s", rec.Code, tc.wantStatus, rec.Body)
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost:8080"+tc.target, nil)
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
