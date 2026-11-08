package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// FuelStore é o que os handlers de combustíveis precisam do banco.
type FuelStore interface {
	ListFuelProducts(ctx context.Context) ([]db.ListFuelProductsRow, error)
	GetFuelProduct(ctx context.Context, id string) (db.MartFuelProduct, error)
	ListFuelPricesWeekly(ctx context.Context, arg db.ListFuelPricesWeeklyParams) ([]db.ListFuelPricesWeeklyRow, error)
	ListFuelMonths(ctx context.Context, productID string) ([]time.Time, error)
	ListFuelPricesMunicipal(ctx context.Context, arg db.ListFuelPricesMunicipalParams) ([]db.ListFuelPricesMunicipalRow, error)
}

// --- Formatos das respostas ---------------------------------------------------

type fuelJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Unit      string    `json:"unit"`
	UpdatedAt time.Time `json:"updated_at"`
	// Média nacional da semana mais recente (só na listagem).
	Latest *weekPriceJSON `json:"latest,omitempty"`
}

type weekPriceJSON struct {
	WeekStart string  `json:"week_start"` // segunda-feira, AAAA-MM-DD
	AvgPrice  float64 `json:"avg_price"`
	NStations int32   `json:"n_stations"`
}

type municipalPriceJSON struct {
	IBGECode  int32   `json:"ibge_code"`
	Name      string  `json:"name"`
	UF        string  `json:"uf"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	AvgPrice  float64 `json:"avg_price"`
	MinPrice  float64 `json:"min_price"`
	MaxPrice  float64 `json:"max_price"`
	NStations int32   `json:"n_stations"`
}

type fuelListResponse struct {
	Data []fuelJSON `json:"data"`
}

type fuelPricesResponse struct {
	Fuel  fuelJSON        `json:"fuel"`
	Scope string          `json:"scope"`
	Data  []weekPriceJSON `json:"data"`
}

type fuelMunicipalResponse struct {
	Fuel fuelJSON `json:"fuel"`
	// Mês mostrado (AAAA-MM). Ponteiro: vira null quando o combustível não
	// tem nenhum mês com dados.
	Month           *string              `json:"month"`
	AvailableMonths []string             `json:"available_months"`
	Data            []municipalPriceJSON `json:"data"`
}

func fuelFromModel(m db.MartFuelProduct) fuelJSON {
	return fuelJSON{ID: m.ID, Name: m.Name, Unit: m.Unit, UpdatedAt: m.UpdatedAt.UTC()}
}

func newWeekPrice(weekStart time.Time, avg float64, n int32) weekPriceJSON {
	return weekPriceJSON{WeekStart: weekStart.Format(time.DateOnly), AvgPrice: avg, NStations: n}
}

const monthLayout = "2006-01"

// --- Handlers -----------------------------------------------------------------

// listFuels responde GET /v1/fuels: os combustíveis com a média nacional da
// semana mais recente.
func (h *handlers) listFuels(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.ListFuelProducts(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	data := make([]fuelJSON, 0, len(rows))
	for _, row := range rows {
		fuel := fuelFromModel(row.MartFuelProduct)
		latest := newWeekPrice(row.LastWeekStart, row.LastAvgPrice, row.LastNStations)
		fuel.Latest = &latest
		data = append(data, fuel)
	}
	writeJSON(w, http.StatusOK, fuelListResponse{Data: data})
}

// listFuelPrices responde GET /v1/fuels/{id}/prices?scope=&from=&to=: a série
// semanal no Brasil (scope=BR, o padrão) ou numa UF.
func (h *handlers) listFuelPrices(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scope, err := parseScope(r.URL.Query().Get("scope"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}
	from, to, err := parseDateRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}

	fuel, ok := h.getFuel(w, r, id)
	if !ok {
		return
	}
	rows, err := h.store.ListFuelPricesWeekly(r.Context(), db.ListFuelPricesWeeklyParams{
		ProductID: id, Scope: scope, FromDate: from, ToDate: to,
	})
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	data := make([]weekPriceJSON, 0, len(rows))
	for _, row := range rows {
		data = append(data, newWeekPrice(row.WeekStart, row.AvgPrice, row.NStations))
	}
	writeJSON(w, http.StatusOK, fuelPricesResponse{Fuel: fuelFromModel(fuel), Scope: scope, Data: data})
}

// listFuelMunicipalities responde GET /v1/fuels/{id}/municipalities?month=:
// o preço em cada município num mês (o mais recente, se month faltar), com as
// coordenadas para o mapa e a lista de meses disponíveis.
func (h *handlers) listFuelMunicipalities(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	month, hasMonth, err := parseMonth(r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}

	fuel, ok := h.getFuel(w, r, id)
	if !ok {
		return
	}
	months, err := h.store.ListFuelMonths(r.Context(), id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	resp := fuelMunicipalResponse{
		Fuel:            fuelFromModel(fuel),
		AvailableMonths: make([]string, 0, len(months)),
		Data:            []municipalPriceJSON{},
	}
	for _, m := range months {
		resp.AvailableMonths = append(resp.AvailableMonths, m.Format(monthLayout))
	}
	if !hasMonth {
		if len(months) == 0 {
			writeJSON(w, http.StatusOK, resp) // sem dados: month fica null
			return
		}
		month = months[len(months)-1]
	}
	shown := month.Format(monthLayout)
	resp.Month = &shown

	// Mês fora da lista: já se sabe que não há dados, sem consultar o banco.
	// Isso também limita o cache a uma entrada por mês que existe, e não uma
	// por mês qualquer que alguém escreva na URL.
	if !slices.ContainsFunc(months, month.Equal) {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	rows, err := h.store.ListFuelPricesMunicipal(r.Context(), db.ListFuelPricesMunicipalParams{
		ProductID: id, Month: month,
	})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, row := range rows {
		resp.Data = append(resp.Data, municipalPriceJSON{
			IBGECode: row.IbgeCode, Name: row.Name, UF: row.Uf,
			Latitude: row.Latitude, Longitude: row.Longitude,
			AvgPrice: row.AvgPrice, MinPrice: row.MinPrice, MaxPrice: row.MaxPrice,
			NStations: row.NStations,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// getFuel busca o combustível. Se ele não existir (404) ou der erro (500),
// já escreve a resposta e devolve false: o handler só precisa sair.
func (h *handlers) getFuel(w http.ResponseWriter, r *http.Request, id string) (db.MartFuelProduct, bool) {
	fuel, err := h.store.GetFuelProduct(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("combustível %q não encontrado", id))
		return fuel, false
	}
	if err != nil {
		h.internalError(w, r, err)
		return fuel, false
	}
	return fuel, true
}

// --- Parâmetros ---------------------------------------------------------------

// scopes válidos: o Brasil e as 27 UFs. Consultar num map uma chave que não
// existe devolve o valor zero do tipo (false), então scopes["XX"] já é false.
var scopes = map[string]bool{
	"BR": true,
	"AC": true, "AL": true, "AM": true, "AP": true, "BA": true, "CE": true, "DF": true,
	"ES": true, "GO": true, "MA": true, "MG": true, "MS": true, "MT": true, "PA": true,
	"PB": true, "PE": true, "PI": true, "PR": true, "RJ": true, "RN": true, "RO": true,
	"RR": true, "RS": true, "SC": true, "SE": true, "SP": true, "TO": true,
}

// parseScope lê ?scope= (padrão "BR"); aceita minúsculas ("sp").
func parseScope(v string) (string, error) {
	if v == "" {
		return "BR", nil
	}
	scope := strings.ToUpper(v)
	if !scopes[scope] {
		return "", fmt.Errorf("parâmetro scope inválido (%q): use BR ou a sigla de uma UF", v)
	}
	return scope, nil
}

// parseMonth lê ?month=AAAA-MM. Devolve ok = false quando o parâmetro falta.
func parseMonth(v string) (month time.Time, ok bool, err error) {
	if v == "" {
		return time.Time{}, false, nil
	}
	month, err = time.Parse(monthLayout, v)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parâmetro month inválido (%q): use AAAA-MM", v)
	}
	return month, true, nil
}
