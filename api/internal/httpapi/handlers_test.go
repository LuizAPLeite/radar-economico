package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// fakeStore faz o papel do banco: devolve o que o teste configurou e
// registra os parâmetros recebidos.
type fakeStore struct {
	indicators []db.ListIndicatorsRow
	listErr    error

	indicator db.MartIndicator
	getErr    error

	values    []db.ListIndicatorValuesRow
	valuesErr error
	gotParams *db.ListIndicatorValuesParams
}

func (f *fakeStore) ListIndicators(context.Context) ([]db.ListIndicatorsRow, error) {
	return f.indicators, f.listErr
}

func (f *fakeStore) GetIndicator(_ context.Context, _ string) (db.MartIndicator, error) {
	return f.indicator, f.getErr
}

func (f *fakeStore) ListIndicatorValues(_ context.Context, p db.ListIndicatorValuesParams) ([]db.ListIndicatorValuesRow, error) {
	f.gotParams = &p
	return f.values, f.valuesErr
}

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

var usdBRL = db.MartIndicator{
	ID: "usd_brl", Name: "Dólar comercial (venda)", Unit: "R$/US$", Frequency: "daily",
	Source: "BCB", SourceCode: "1", SourceRunID: 14,
	UpdatedAt: time.Date(2026, 11, 7, 18, 39, 18, 0, time.UTC),
}

const usdBRLJSON = `{"id":"usd_brl","name":"Dólar comercial (venda)","unit":"R$/US$",` +
	`"frequency":"daily","source":"BCB","source_code":"1","updated_at":"2026-11-07T18:39:18Z"`

const testSpec = "openapi: 3.1.0\n"

// do executa uma requisição no router completo (com middlewares) e devolve
// a resposta gravada. Nenhuma porta de rede é aberta.
func do(t *testing.T, store Store, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	router := NewRouter(store, slog.New(slog.NewTextHandler(io.Discard, nil)), []byte(testSpec))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

// assertJSON confere status, Content-Type e o corpo exato da resposta.
// Comparar o JSON inteiro testa o contrato da API: nome, ordem e formato
// de cada campo.
func assertJSON(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Errorf("status = %d, quer %d", rec.Code, wantStatus)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got := rec.Body.String(); got != wantBody {
		t.Errorf("corpo:\n got: %s\nwant: %s", got, wantBody)
	}
}

// errorCode extrai o campo error.code de uma resposta de erro.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body errorJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é um erro JSON: %v\n%s", err, rec.Body)
	}
	return body.Error.Code
}

func TestHealth(t *testing.T) {
	assertJSON(t, do(t, &fakeStore{}, http.MethodGet, "/health"), http.StatusOK, `{"status":"ok"}`)
}

func TestOpenAPISpec(t *testing.T) {
	rec := do(t, &fakeStore{}, http.MethodGet, "/openapi.yaml")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/yaml" || rec.Body.String() != testSpec {
		t.Errorf("status = %d, Content-Type = %q, corpo = %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
}

func TestRoutingErrorsAreJSON(t *testing.T) {
	tests := []struct {
		name, method, target string
		wantStatus           int
		wantCode             string
	}{
		{"rota inexistente", http.MethodGet, "/v2/nada", http.StatusNotFound, "not_found"},
		{"método não permitido", http.MethodPost, "/health", http.StatusMethodNotAllowed, "method_not_allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, &fakeStore{}, tt.method, tt.target)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, quer %d", rec.Code, tt.wantStatus)
			}
			if code := errorCode(t, rec); code != tt.wantCode {
				t.Errorf("code = %q, quer %q", code, tt.wantCode)
			}
		})
	}
}

func TestListIndicatorValues(t *testing.T) {
	store := &fakeStore{
		indicator: usdBRL,
		values: []db.ListIndicatorValuesRow{
			{RefDate: d(2026, 11, 5), Value: 5.4621},
			{RefDate: d(2026, 11, 6), Value: 5.5013},
		},
	}

	rec := do(t, store, http.MethodGet, "/v1/indicators/usd_brl/values?from=2026-11-01&to=2026-11-07")

	assertJSON(t, rec, http.StatusOK, `{"indicator":`+usdBRLJSON+`},"data":[`+
		`{"date":"2026-11-05","value":5.4621},{"date":"2026-11-06","value":5.5013}]}`)

	p := store.gotParams
	if p == nil || p.IndicatorID != "usd_brl" || !p.FromDate.Equal(d(2026, 11, 1)) || !p.ToDate.Equal(d(2026, 11, 7)) {
		t.Errorf("parâmetros = %+v", p)
	}
}

func TestListIndicatorValues_WithoutRangeUsesWholeSeries(t *testing.T) {
	store := &fakeStore{indicator: usdBRL}
	do(t, store, http.MethodGet, "/v1/indicators/usd_brl/values")

	p := store.gotParams
	if p == nil || !p.FromDate.Equal(minDate) || !p.ToDate.Equal(maxDate) {
		t.Errorf("parâmetros = %+v", p)
	}
}

func TestListIndicatorValues_EmptyRangeIsEmptyArray(t *testing.T) {
	rec := do(t, &fakeStore{indicator: usdBRL}, http.MethodGet, "/v1/indicators/usd_brl/values?from=2030-01-01")
	assertJSON(t, rec, http.StatusOK, `{"indicator":`+usdBRLJSON+`},"data":[]}`)
}

func TestListIndicatorValues_InvalidParams(t *testing.T) {
	for name, query := range map[string]string{
		"data em outro formato": "?from=07/11/2026",
		"data que não existe":   "?to=2026-02-30",
		"from depois de to":     "?from=2026-11-07&to=2026-11-01",
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{indicator: usdBRL}
			rec := do(t, store, http.MethodGet, "/v1/indicators/usd_brl/values"+query)

			if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_parameter" {
				t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
			}
			if store.gotParams != nil {
				t.Error("não deveria consultar o banco com parâmetros inválidos")
			}
		})
	}
}

func TestListIndicatorValues_UnknownIndicator(t *testing.T) {
	rec := do(t, &fakeStore{getErr: pgx.ErrNoRows}, http.MethodGet, "/v1/indicators/nao_existe/values")

	if rec.Code != http.StatusNotFound || errorCode(t, rec) != "not_found" {
		t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "nao_existe") {
		t.Errorf("a mensagem deveria citar o id pedido: %s", rec.Body)
	}
}

func TestListIndicatorValues_DatabaseErrorIsNotLeaked(t *testing.T) {
	store := &fakeStore{indicator: usdBRL, valuesErr: errors.New(`relation "mart.indicator_values" does not exist`)}
	rec := do(t, store, http.MethodGet, "/v1/indicators/usd_brl/values")

	if rec.Code != http.StatusInternalServerError || errorCode(t, rec) != "internal" {
		t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "relation") {
		t.Errorf("detalhe interno vazou para o cliente: %s", rec.Body)
	}
}

func TestCompressesWhenClientAccepts(t *testing.T) {
	router := NewRouter(&fakeStore{indicator: usdBRL}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/indicators/usd_brl/values", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, quer gzip", got)
	}
}
