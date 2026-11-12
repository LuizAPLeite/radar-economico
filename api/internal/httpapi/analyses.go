package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// AnalysisStore é o que os handlers das análises precisam do banco.
type AnalysisStore interface {
	ListAnalysisRegimes(ctx context.Context) ([]db.MartAnalysisRegime, error)
	ListLagCorrelations(ctx context.Context, productID string) ([]db.ListLagCorrelationsRow, error)
	ListRollingCorrelations(ctx context.Context, productID string) ([]db.ListRollingCorrelationsRow, error)
	ListFuelForecasts(ctx context.Context, productID string) ([]db.ListFuelForecastsRow, error)
	ListForecastBacktests(ctx context.Context, productID string) ([]db.ListForecastBacktestsRow, error)
	ListOfficialFuelPrices(ctx context.Context, arg db.ListOfficialFuelPricesParams) ([]db.ListOfficialFuelPricesRow, error)
}

// historyWeeks é quanto da série oficial vem junto com a previsão, para o
// gráfico mostrar o passado recente ao lado das semanas previstas.
const historyWeeks = 104

// --- Formatos das respostas ---------------------------------------------------

type regimeJSON struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	StartsOn string  `json:"starts_on"`
	EndsOn   *string `json:"ends_on"` // null: período atual
	// Explicação curta, em português, para mostrar na página.
	Description string `json:"description"`
}

type lagCorrelationJSON struct {
	Driver      string  `json:"driver"`
	RegimeID    string  `json:"regime_id"`
	LagWeeks    int32   `json:"lag_weeks"`
	Correlation float64 `json:"correlation"`
	NWeeks      int32   `json:"n_weeks"`
}

type rollingCorrelationJSON struct {
	Driver      string  `json:"driver"`
	WeekStart   string  `json:"week_start"`
	LagWeeks    int32   `json:"lag_weeks"`
	Correlation float64 `json:"correlation"`
}

type passThroughResponse struct {
	Fuel    fuelJSON                 `json:"fuel"`
	Regimes []regimeJSON             `json:"regimes"`
	Lags    []lagCorrelationJSON     `json:"lags"`
	Rolling []rollingCorrelationJSON `json:"rolling"`
}

type forecastPointJSON struct {
	WeekStart    string  `json:"week_start"`
	HorizonWeeks int32   `json:"horizon_weeks"`
	Predicted    float64 `json:"predicted"`
	Lower80      float64 `json:"lower_80"`
	Upper80      float64 `json:"upper_80"`
}

type backtestJSON struct {
	Model        string  `json:"model"`
	HorizonWeeks int32   `json:"horizon_weeks"`
	MAE          float64 `json:"mae"`
	MAPE         float64 `json:"mape"`
	NForecasts   int32   `json:"n_forecasts"`
}

type forecastResponse struct {
	Fuel fuelJSON `json:"fuel"`
	// Modelo publicado e última semana observada; null sem previsão.
	Model     *string             `json:"model"`
	BaseWeek  *string             `json:"base_week"`
	History   []weekPriceJSON     `json:"history"`
	Forecasts []forecastPointJSON `json:"forecasts"`
	Backtests []backtestJSON      `json:"backtests"`
}

// --- Handlers -----------------------------------------------------------------

// getPassThrough responde GET /v1/fuels/{id}/pass-through: quanto a variação
// do preço na bomba acompanha a do dólar e a do petróleo, por período da
// política de preços e por defasagem, e a correlação móvel de 52 semanas.
func (h *handlers) getPassThrough(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fuel, ok := h.getFuel(w, r, id)
	if !ok {
		return
	}
	regimes, err := h.store.ListAnalysisRegimes(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	lags, err := h.store.ListLagCorrelations(r.Context(), id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	rolling, err := h.store.ListRollingCorrelations(r.Context(), id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	resp := passThroughResponse{
		Fuel:    fuelFromModel(fuel),
		Regimes: make([]regimeJSON, 0, len(regimes)),
		Lags:    make([]lagCorrelationJSON, 0, len(lags)),
		Rolling: make([]rollingCorrelationJSON, 0, len(rolling)),
	}
	for _, rg := range regimes {
		resp.Regimes = append(resp.Regimes, regimeJSON{
			ID: rg.ID, Name: rg.Name, StartsOn: rg.StartsOn.Format(time.DateOnly),
			EndsOn: optionalDate(rg.EndsOn.Time, rg.EndsOn.Valid), Description: rg.Description,
		})
	}
	for _, l := range lags {
		resp.Lags = append(resp.Lags, lagCorrelationJSON{
			Driver: l.Driver, RegimeID: l.RegimeID, LagWeeks: l.LagWeeks,
			Correlation: l.Correlation, NWeeks: l.NWeeks,
		})
	}
	for _, rc := range rolling {
		resp.Rolling = append(resp.Rolling, rollingCorrelationJSON{
			Driver: rc.Driver, WeekStart: rc.WeekStart.Format(time.DateOnly),
			LagWeeks: rc.LagWeeks, Correlation: rc.Correlation,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// getForecast responde GET /v1/fuels/{id}/forecast: a previsão das próximas
// semanas com o intervalo de 80%, o erro de cada modelo no backtest e as
// últimas semanas da série oficial da ANP.
func (h *handlers) getForecast(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fuel, ok := h.getFuel(w, r, id)
	if !ok {
		return
	}
	forecasts, err := h.store.ListFuelForecasts(r.Context(), id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	backtests, err := h.store.ListForecastBacktests(r.Context(), id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	history, err := h.store.ListOfficialFuelPrices(r.Context(), db.ListOfficialFuelPricesParams{
		ProductID: id, FromDate: MinDate, ToDate: MaxDate,
	})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if len(history) > historyWeeks {
		history = history[len(history)-historyWeeks:]
	}

	resp := forecastResponse{
		Fuel:      fuelFromModel(fuel),
		History:   make([]weekPriceJSON, 0, len(history)),
		Forecasts: make([]forecastPointJSON, 0, len(forecasts)),
		Backtests: make([]backtestJSON, 0, len(backtests)),
	}
	if len(forecasts) > 0 {
		resp.Model = &forecasts[0].Model
		resp.BaseWeek = optionalDate(forecasts[0].BaseWeek, true)
	}
	for _, p := range history {
		resp.History = append(resp.History, newWeekPrice(p.WeekStart, p.AvgPrice, p.NStations))
	}
	for _, f := range forecasts {
		resp.Forecasts = append(resp.Forecasts, forecastPointJSON{
			WeekStart: f.WeekStart.Format(time.DateOnly), HorizonWeeks: f.HorizonWeeks,
			Predicted: f.Predicted, Lower80: f.Lower80, Upper80: f.Upper80,
		})
	}
	for _, b := range backtests {
		resp.Backtests = append(resp.Backtests, backtestJSON{
			Model: b.Model, HorizonWeeks: b.HorizonWeeks, MAE: b.Mae, MAPE: b.Mape, NForecasts: b.NForecasts,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// optionalDate formata a data como AAAA-MM-DD, ou devolve nil (null no JSON).
func optionalDate(t time.Time, valid bool) *string {
	if !valid {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}
