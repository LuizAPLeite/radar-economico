package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// Testes de GET /v1/indicators: descrevem o que listIndicators deve fazer.

var ipca = db.MartIndicator{
	ID: "ipca", Name: "IPCA (variação mensal)", Unit: "% a.m.", Frequency: "monthly",
	Source: "BCB", SourceCode: "433", SourceRunID: 14,
	UpdatedAt: time.Date(2026, 11, 7, 18, 39, 18, 0, time.UTC),
}

func TestListIndicators(t *testing.T) {
	store := &fakeStore{indicators: []db.ListIndicatorsRow{
		{MartIndicator: ipca, LastDate: d(2026, 9, 1), LastValue: 0.82},
		{MartIndicator: usdBRL, LastDate: d(2026, 11, 6), LastValue: 5.5013},
	}}

	rec := do(t, store, http.MethodGet, "/v1/indicators")

	assertJSON(t, rec, http.StatusOK, `{"data":[`+
		`{"id":"ipca","name":"IPCA (variação mensal)","unit":"% a.m.","frequency":"monthly",`+
		`"source":"BCB","source_code":"433","updated_at":"2026-11-07T18:39:18Z",`+
		`"latest":{"date":"2026-09-01","value":0.82}},`+
		usdBRLJSON+`,"latest":{"date":"2026-11-06","value":5.5013}}]}`)
}

// Sem indicadores, "data" precisa ser uma lista vazia ([]), e não null.
// Dica: é a diferença entre `var lista []indicatorJSON` e make(..., 0, ...).
func TestListIndicators_EmptyIsEmptyArray(t *testing.T) {
	rec := do(t, &fakeStore{}, http.MethodGet, "/v1/indicators")
	assertJSON(t, rec, http.StatusOK, `{"data":[]}`)
}

func TestListIndicators_DatabaseErrorIsNotLeaked(t *testing.T) {
	rec := do(t, &fakeStore{listErr: errors.New("conexão recusada em 10.0.0.5")}, http.MethodGet, "/v1/indicators")

	if rec.Code != http.StatusInternalServerError || errorCode(t, rec) != "internal" {
		t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("detalhe interno vazou para o cliente: %s", rec.Body)
	}
}
