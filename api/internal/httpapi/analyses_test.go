package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// fakeAnalysisStore implementa AnalysisStore para os testes.
type fakeAnalysisStore struct {
	regimes     []db.MartAnalysisRegime
	lags        []db.ListLagCorrelationsRow
	rolling     []db.ListRollingCorrelationsRow
	forecasts   []db.ListFuelForecastsRow
	backtests   []db.ListForecastBacktestsRow
	official    []db.ListOfficialFuelPricesRow
	analysisErr error
}

func (f *fakeAnalysisStore) ListAnalysisRegimes(context.Context) ([]db.MartAnalysisRegime, error) {
	return f.regimes, f.analysisErr
}

func (f *fakeAnalysisStore) ListLagCorrelations(context.Context, string) ([]db.ListLagCorrelationsRow, error) {
	return f.lags, f.analysisErr
}

func (f *fakeAnalysisStore) ListRollingCorrelations(context.Context, string) ([]db.ListRollingCorrelationsRow, error) {
	return f.rolling, f.analysisErr
}

func (f *fakeAnalysisStore) ListFuelForecasts(context.Context, string) ([]db.ListFuelForecastsRow, error) {
	return f.forecasts, f.analysisErr
}

func (f *fakeAnalysisStore) ListForecastBacktests(context.Context, string) ([]db.ListForecastBacktestsRow, error) {
	return f.backtests, f.analysisErr
}

func (f *fakeAnalysisStore) ListOfficialFuelPrices(context.Context, db.ListOfficialFuelPricesParams) ([]db.ListOfficialFuelPricesRow, error) {
	return f.official, f.analysisErr
}

var sampleAnalyses = fakeAnalysisStore{
	regimes: []db.MartAnalysisRegime{
		{ID: "ppi", Name: "Paridade", StartsOn: d(2016, 10, 14), EndsOn: pgtype.Date{Time: d(2023, 5, 16), Valid: true}, Description: "PPI"},
		{ID: "atual", Name: "Atual", StartsOn: d(2023, 5, 16), Description: "Hoje"},
	},
	lags:    []db.ListLagCorrelationsRow{{Driver: "brent_brl", RegimeID: "atual", LagWeeks: 2, Correlation: 0.41, NWeeks: 181}},
	rolling: []db.ListRollingCorrelationsRow{{Driver: "brent_brl", WeekStart: d(2026, 10, 26), LagWeeks: 2, Correlation: 0.38}},
	forecasts: []db.ListFuelForecastsRow{{
		WeekStart: d(2026, 11, 9), Model: "naive", BaseWeek: d(2026, 11, 2), HorizonWeeks: 1,
		Predicted: 6.47, Lower80: 6.449, Upper80: 6.491,
	}},
	backtests: []db.ListForecastBacktestsRow{{Model: "naive", HorizonWeeks: 1, Mae: 0.019, Mape: 0.3, NForecasts: 104}},
	official:  []db.ListOfficialFuelPricesRow{{WeekStart: d(2026, 11, 2), AvgPrice: 6.47, NStations: 4300}},
}

func analysisStore(a fakeAnalysisStore) *fakeStore {
	return &fakeStore{fakeFuelStore: fakeFuelStore{fuel: gasolina}, fakeAnalysisStore: a}
}

func TestGetPassThrough(t *testing.T) {
	rec := do(t, analysisStore(sampleAnalyses), http.MethodGet, "/v1/fuels/gasolina/pass-through")
	assertJSON(t, rec, http.StatusOK, `{"fuel":`+gasolinaJSON+`},`+
		`"regimes":[`+
		`{"id":"ppi","name":"Paridade","starts_on":"2016-10-14","ends_on":"2023-05-16","description":"PPI"},`+
		`{"id":"atual","name":"Atual","starts_on":"2023-05-16","ends_on":null,"description":"Hoje"}],`+
		`"lags":[{"driver":"brent_brl","regime_id":"atual","lag_weeks":2,"correlation":0.41,"n_weeks":181}],`+
		`"rolling":[{"driver":"brent_brl","week_start":"2026-10-26","lag_weeks":2,"correlation":0.38}]}`)
}

func TestGetForecast(t *testing.T) {
	rec := do(t, analysisStore(sampleAnalyses), http.MethodGet, "/v1/fuels/gasolina/forecast")
	assertJSON(t, rec, http.StatusOK, `{"fuel":`+gasolinaJSON+`},"model":"naive","base_week":"2026-11-02",`+
		`"history":[{"week_start":"2026-11-02","avg_price":6.47,"n_stations":4300}],`+
		`"forecasts":[{"week_start":"2026-11-09","horizon_weeks":1,"predicted":6.47,"lower_80":6.449,"upper_80":6.491}],`+
		`"backtests":[{"model":"naive","horizon_weeks":1,"mae":0.019,"mape":0.3,"n_forecasts":104}]}`)
}

func TestForecastWithoutAnalysisHasEmptyListsAndNulls(t *testing.T) {
	rec := do(t, analysisStore(fakeAnalysisStore{}), http.MethodGet, "/v1/fuels/gasolina/forecast")
	assertJSON(t, rec, http.StatusOK, `{"fuel":`+gasolinaJSON+`},"model":null,"base_week":null,`+
		`"history":[],"forecasts":[],"backtests":[]}`)
}

func TestForecastHistoryIsLimitedToTheLastWeeks(t *testing.T) {
	store := analysisStore(fakeAnalysisStore{})
	for i := range historyWeeks + 10 {
		store.official = append(store.official, db.ListOfficialFuelPricesRow{
			WeekStart: d(2020, 1, 6).AddDate(0, 0, 7*i), AvgPrice: 5, NStations: 1,
		})
	}
	rec := do(t, store, http.MethodGet, "/v1/fuels/gasolina/forecast")
	var body struct {
		History []weekPriceJSON `json:"history"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.History) != historyWeeks {
		t.Fatalf("history com %d semanas, quer %d", len(body.History), historyWeeks)
	}
	if last := store.official[len(store.official)-1].WeekStart.Format("2006-01-02"); body.History[historyWeeks-1].WeekStart != last {
		t.Errorf("última semana = %s, quer %s", body.History[historyWeeks-1].WeekStart, last)
	}
}

func TestAnalysesOfUnknownFuelAre404AndErrorsAre500(t *testing.T) {
	for _, path := range []string{"/v1/fuels/querosene/pass-through", "/v1/fuels/querosene/forecast"} {
		store := analysisStore(sampleAnalyses)
		store.fuelErr = pgx.ErrNoRows
		assertJSON(t, do(t, store, http.MethodGet, path), http.StatusNotFound,
			`{"error":{"code":"not_found","message":"combustível \"querosene\" não encontrado"}}`)
	}
	for _, path := range []string{"/v1/fuels/gasolina/pass-through", "/v1/fuels/gasolina/forecast"} {
		broken := sampleAnalyses
		broken.analysisErr = errors.New("conexão recusada")
		rec := do(t, analysisStore(broken), http.MethodGet, path)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, quer 500", path, rec.Code)
		}
	}
}
