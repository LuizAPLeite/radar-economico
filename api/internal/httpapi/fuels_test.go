package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// fakeFuelStore implementa FuelStore para os testes.
type fakeFuelStore struct {
	fuels      []db.ListFuelProductsRow
	fuelsErr   error
	fuel       db.MartFuelProduct
	fuelErr    error
	weekly     []db.ListFuelPricesWeeklyRow
	gotWeekly  *db.ListFuelPricesWeeklyParams
	months     []time.Time
	municipal  []db.ListFuelPricesMunicipalRow
	gotMonthly *db.ListFuelPricesMunicipalParams
}

func (f *fakeFuelStore) ListFuelProducts(context.Context) ([]db.ListFuelProductsRow, error) {
	return f.fuels, f.fuelsErr
}

func (f *fakeFuelStore) GetFuelProduct(context.Context, string) (db.MartFuelProduct, error) {
	return f.fuel, f.fuelErr
}

func (f *fakeFuelStore) ListFuelPricesWeekly(_ context.Context, p db.ListFuelPricesWeeklyParams) ([]db.ListFuelPricesWeeklyRow, error) {
	f.gotWeekly = &p
	return f.weekly, nil
}

func (f *fakeFuelStore) ListFuelMonths(context.Context, string) ([]time.Time, error) {
	return f.months, nil
}

func (f *fakeFuelStore) ListFuelPricesMunicipal(_ context.Context, p db.ListFuelPricesMunicipalParams) ([]db.ListFuelPricesMunicipalRow, error) {
	f.gotMonthly = &p
	return f.municipal, nil
}

var gasolina = db.MartFuelProduct{
	ID: "gasolina", Name: "Gasolina comum", Unit: "R$/l",
	UpdatedAt: time.Date(2026, 11, 7, 19, 0, 0, 0, time.UTC),
}

const gasolinaJSON = `{"id":"gasolina","name":"Gasolina comum","unit":"R$/l","updated_at":"2026-11-07T19:00:00Z"`

func fuelStore(f fakeFuelStore) *fakeStore { return &fakeStore{fakeFuelStore: f} }

func TestListFuels(t *testing.T) {
	store := fuelStore(fakeFuelStore{fuels: []db.ListFuelProductsRow{
		{MartFuelProduct: gasolina, LastWeekStart: d(2026, 10, 26), LastAvgPrice: 6.505, LastNStations: 4560},
	}})

	rec := do(t, store, http.MethodGet, "/v1/fuels")

	assertJSON(t, rec, http.StatusOK, `{"data":[`+gasolinaJSON+
		`,"latest":{"week_start":"2026-10-26","avg_price":6.505,"n_stations":4560}}]}`)
}

func TestListFuels_EmptyIsEmptyArray(t *testing.T) {
	assertJSON(t, do(t, fuelStore(fakeFuelStore{}), http.MethodGet, "/v1/fuels"), http.StatusOK, `{"data":[]}`)
}

func TestListFuelPrices(t *testing.T) {
	store := fuelStore(fakeFuelStore{
		fuel: gasolina,
		weekly: []db.ListFuelPricesWeeklyRow{
			{WeekStart: d(2026, 10, 19), AvgPrice: 6.29, NStations: 912},
			{WeekStart: d(2026, 10, 26), AvgPrice: 6.27, NStations: 900},
		},
	})

	// "sp" minúsculo é aceito e normalizado.
	rec := do(t, store, http.MethodGet, "/v1/fuels/gasolina/prices?scope=sp&from=2026-10-01")

	assertJSON(t, rec, http.StatusOK, `{"fuel":`+gasolinaJSON+`},"scope":"SP","data":[`+
		`{"week_start":"2026-10-19","avg_price":6.29,"n_stations":912},`+
		`{"week_start":"2026-10-26","avg_price":6.27,"n_stations":900}]}`)
	p := store.gotWeekly
	if p == nil || p.ProductID != "gasolina" || p.Scope != "SP" || !p.FromDate.Equal(d(2026, 10, 1)) || !p.ToDate.Equal(MaxDate) {
		t.Errorf("parâmetros = %+v", p)
	}
}

func TestListFuelPrices_DefaultScopeIsBrazil(t *testing.T) {
	store := fuelStore(fakeFuelStore{fuel: gasolina})
	rec := do(t, store, http.MethodGet, "/v1/fuels/gasolina/prices")

	if rec.Code != http.StatusOK || store.gotWeekly == nil || store.gotWeekly.Scope != "BR" {
		t.Errorf("status = %d, parâmetros = %+v", rec.Code, store.gotWeekly)
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("série vazia deveria ser []: %s", rec.Body)
	}
}

func TestFuelRoutes_InvalidParams(t *testing.T) {
	for name, target := range map[string]string{
		"UF inexistente":           "/v1/fuels/gasolina/prices?scope=XX",
		"data em outro formato":    "/v1/fuels/gasolina/prices?from=07/11/2026",
		"mês em outro formato":     "/v1/fuels/gasolina/municipalities?month=10/2026",
		"mês que não existe":       "/v1/fuels/gasolina/municipalities?month=2026-13",
		"mês com dia (AAAA-MM-DD)": "/v1/fuels/gasolina/municipalities?month=2026-10-01",
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, fuelStore(fakeFuelStore{fuel: gasolina}), http.MethodGet, target)
			if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_parameter" {
				t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestFuelRoutes_UnknownFuel(t *testing.T) {
	for _, target := range []string{"/v1/fuels/querosene/prices", "/v1/fuels/querosene/municipalities"} {
		rec := do(t, fuelStore(fakeFuelStore{fuelErr: pgx.ErrNoRows}), http.MethodGet, target)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "querosene") {
			t.Errorf("%s: status = %d, corpo = %s", target, rec.Code, rec.Body)
		}
	}
}

func TestListFuels_DatabaseErrorIsNotLeaked(t *testing.T) {
	rec := do(t, fuelStore(fakeFuelStore{fuelsErr: errors.New("senha incorreta para o usuário radar")}), http.MethodGet, "/v1/fuels")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "senha") {
		t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
	}
}

var monthsSepOct = []time.Time{d(2026, 9, 1), d(2026, 10, 1)}

func TestListFuelMunicipalities_DefaultsToLatestMonth(t *testing.T) {
	store := fuelStore(fakeFuelStore{
		fuel:   gasolina,
		months: monthsSepOct,
		municipal: []db.ListFuelPricesMunicipalRow{{
			IbgeCode: 2903201, Name: "Barreiras", Uf: "BA", Latitude: -12.013, Longitude: -45.5424,
			AvgPrice: 6.89, MinPrice: 6.79, MaxPrice: 7.09, NStations: 9,
		}},
	})

	rec := do(t, store, http.MethodGet, "/v1/fuels/gasolina/municipalities")

	assertJSON(t, rec, http.StatusOK, `{"fuel":`+gasolinaJSON+`},"month":"2026-10",`+
		`"available_months":["2026-09","2026-10"],"data":[{"ibge_code":2903201,"name":"Barreiras",`+
		`"uf":"BA","latitude":-12.013,"longitude":-45.5424,"avg_price":6.89,"min_price":6.79,`+
		`"max_price":7.09,"n_stations":9}]}`)
	if p := store.gotMonthly; p == nil || !p.Month.Equal(d(2026, 10, 1)) {
		t.Errorf("parâmetros = %+v", p)
	}
}

func TestListFuelMunicipalities_ExplicitMonth(t *testing.T) {
	store := fuelStore(fakeFuelStore{fuel: gasolina, months: monthsSepOct})
	rec := do(t, store, http.MethodGet, "/v1/fuels/gasolina/municipalities?month=2026-09")

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"month":"2026-09"`) {
		t.Errorf("status = %d, corpo = %s", rec.Code, rec.Body)
	}
	if p := store.gotMonthly; p == nil || !p.Month.Equal(d(2026, 9, 1)) {
		t.Errorf("parâmetros = %+v", p)
	}
}

func TestListFuelMunicipalities_MonthWithoutData(t *testing.T) {
	store := fuelStore(fakeFuelStore{fuel: gasolina, months: monthsSepOct})
	rec := do(t, store, http.MethodGet, "/v1/fuels/gasolina/municipalities?month=2025-01")

	assertJSON(t, rec, http.StatusOK, `{"fuel":`+gasolinaJSON+`},"month":"2025-01",`+
		`"available_months":["2026-09","2026-10"],"data":[]}`)
	// Mês fora da lista: a resposta sai sem consultar o banco.
	if store.gotMonthly != nil {
		t.Errorf("consultou o banco com %+v", store.gotMonthly)
	}
}

func TestListFuelMunicipalities_NoDataAtAll(t *testing.T) {
	rec := do(t, fuelStore(fakeFuelStore{fuel: gasolina}), http.MethodGet, "/v1/fuels/gasolina/municipalities")

	assertJSON(t, rec, http.StatusOK,
		`{"fuel":`+gasolinaJSON+`},"month":null,"available_months":[],"data":[]}`)
}
