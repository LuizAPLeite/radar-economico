package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// fakeHealthStore implementa HealthStore para os testes.
type fakeHealthStore struct {
	freshness    []db.ListDatasetFreshnessRow
	freshnessErr error
	lastRuns     []db.ListLastRunsRow
	lastOK       []db.ListLastSuccessfulRunsRow
}

func (f *fakeHealthStore) ListDatasetFreshness(context.Context) ([]db.ListDatasetFreshnessRow, error) {
	return f.freshness, f.freshnessErr
}

func (f *fakeHealthStore) ListLastRuns(context.Context) ([]db.ListLastRunsRow, error) {
	return f.lastRuns, nil
}

func (f *fakeHealthStore) ListLastSuccessfulRuns(context.Context) ([]db.ListLastSuccessfulRunsRow, error) {
	return f.lastOK, nil
}

// Domingo, 8/11/2026, às 10h em Brasília (13h UTC).
var sunday = time.Date(2026, 11, 8, 13, 0, 0, 0, time.UTC)

// Os dados reais do banco de desenvolvimento nesse domingo.
var freshData = []db.ListDatasetFreshnessRow{
	{Dataset: "ipca", Latest: d(2026, 9, 1)},     // 68 dias: o de outubro sai por volta do dia 10
	{Dataset: "selic", Latest: d(2026, 11, 8)},   // a meta Selic tem valor todo dia
	{Dataset: "usd_brl", Latest: d(2026, 11, 6)}, // sexta-feira
	{Dataset: "fuels", Latest: d(2026, 10, 26)},
}

func ts(hour int) time.Time { return time.Date(2026, 11, 8, hour, 0, 0, 0, time.UTC) }

func TestBuildHealthReport_AllFresh(t *testing.T) {
	report, err := buildHealthReport(sunday, freshData, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ok" {
		t.Errorf("status = %q, quer ok", report.Status)
	}
	got := map[string]int{}
	for _, ds := range report.Datasets {
		if ds.Status != "ok" || ds.AgeDays == nil {
			t.Errorf("%s: %+v", ds.ID, ds)
			continue
		}
		got[ds.ID] = *ds.AgeDays
	}
	want := map[string]int{"selic": 0, "usd_brl": 2, "ipca": 68, "fuels": 13}
	for id, age := range want {
		if got[id] != age {
			t.Errorf("%s: %d dias, quer %d", id, got[id], age)
		}
	}
}

func TestBuildHealthReport_TodayIsBrasiliaTime(t *testing.T) {
	// 01h30 UTC de 9/11 ainda é 22h30 de 8/11 em Brasília: o dólar de sexta
	// tem 2 dias, não 3.
	at := time.Date(2026, 11, 9, 1, 30, 0, 0, time.UTC)
	report, _ := buildHealthReport(at, freshData, nil, nil)
	for _, ds := range report.Datasets {
		if ds.ID == "usd_brl" && *ds.AgeDays != 2 {
			t.Errorf("idade do dólar = %d, quer 2", *ds.AgeDays)
		}
	}
}

func TestBuildHealthReport_StaleAndMissing(t *testing.T) {
	data := []db.ListDatasetFreshnessRow{
		{Dataset: "selic", Latest: d(2026, 11, 8)},
		{Dataset: "usd_brl", Latest: d(2026, 11, 2)}, // 6 dias: passou do prazo de 5
		{Dataset: "fuels", Latest: d(2026, 10, 26)},
		// sem IPCA nenhum
	}
	report, err := buildHealthReport(sunday, data, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "degraded" {
		t.Errorf("status = %q, quer degraded", report.Status)
	}
	status := map[string]string{}
	for _, ds := range report.Datasets {
		status[ds.ID] = ds.Status
	}
	if status["usd_brl"] != "stale" || status["ipca"] != "missing" || status["selic"] != "ok" {
		t.Errorf("status por conjunto = %v", status)
	}
}

func TestBuildHealthReport_Jobs(t *testing.T) {
	lastRuns := []db.ListLastRunsRow{
		{Job: "anp", Status: "skipped", FinishedAt: ts(12)},
		{Job: "bcb", Status: "failed", FinishedAt: ts(12)},
	}
	lastOK := []db.ListLastSuccessfulRunsRow{{
		Job:               "anp",
		FinishedAt:        ts(9),
		SourceCollectedAt: pgtype.Timestamptz{Time: ts(8), Valid: true},
		Warnings:          []byte(`["Gasolina comum em 11/2026: 4 preços"]`),
	}}

	report, err := buildHealthReport(sunday, freshData, lastRuns, lastOK)
	if err != nil {
		t.Fatal(err)
	}

	// A última execução do bcb falhou: o relatório inteiro fica degradado.
	if report.Status != "degraded" {
		t.Errorf("status = %q, quer degraded", report.Status)
	}
	anp, bcb := report.Jobs[0], report.Jobs[1]
	// Os avisos da última carga da ANP aparecem mesmo depois de um skipped.
	if anp.LastStatus != "skipped" || len(anp.Warnings) != 1 || !anp.LastSuccessAt.Equal(ts(9)) || !anp.CollectedAt.Equal(ts(8)) {
		t.Errorf("anp = %+v", anp)
	}
	// O bcb nunca deu certo: sem carga, sem avisos (lista vazia, não null).
	if bcb.LastSuccessAt != nil || bcb.CollectedAt != nil || bcb.Warnings == nil || len(bcb.Warnings) != 0 {
		t.Errorf("bcb = %+v", bcb)
	}
}

func TestHealthData(t *testing.T) {
	now = func() time.Time { return sunday }
	t.Cleanup(func() { now = time.Now })

	store := &fakeStore{fakeHealthStore: fakeHealthStore{
		freshness: freshData[:1], // só o IPCA
		lastRuns:  []db.ListLastRunsRow{{Job: "bcb", Status: "success", FinishedAt: ts(12)}},
		lastOK: []db.ListLastSuccessfulRunsRow{{
			Job: "bcb", FinishedAt: ts(12), Warnings: []byte(`[]`),
			SourceCollectedAt: pgtype.Timestamptz{Time: ts(11), Valid: true},
		}},
	}}
	rec := do(t, store, http.MethodGet, "/health/data")

	// Faltam três conjuntos: 503, com o corpo dizendo quais.
	assertJSON(t, rec, http.StatusServiceUnavailable, `{"status":"degraded","checked_at":"2026-11-08T13:00:00Z",`+
		`"datasets":[`+
		`{"id":"selic","name":"Meta Selic","latest":null,"age_days":null,"max_age_days":5,"status":"missing"},`+
		`{"id":"usd_brl","name":"Dólar comercial (venda)","latest":null,"age_days":null,"max_age_days":5,"status":"missing"},`+
		`{"id":"ipca","name":"IPCA (variação mensal)","latest":"2026-09-01","age_days":68,"max_age_days":75,"status":"ok"},`+
		`{"id":"fuels","name":"Preços de combustíveis (ANP)","latest":null,"age_days":null,"max_age_days":60,"status":"missing"}],`+
		`"jobs":[{"job":"bcb","last_status":"success","last_run_at":"2026-11-08T12:00:00Z",`+
		`"last_success_at":"2026-11-08T12:00:00Z","collected_at":"2026-11-08T11:00:00Z","warnings":[]}]}`)
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, quer no-store", cc)
	}

	// Com tudo em dia, 200.
	store.freshness = freshData
	if rec := do(t, store, http.MethodGet, "/health/data"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
	}
}

func TestHealthData_DatabaseErrorIsNotExposed(t *testing.T) {
	store := &fakeStore{fakeHealthStore: fakeHealthStore{freshnessErr: errors.New("senha incorreta para radar")}}
	rec := do(t, store, http.MethodGet, "/health/data")

	assertJSON(t, rec, http.StatusInternalServerError,
		`{"error":{"code":"internal","message":"erro interno; tente novamente"}}`)
}
