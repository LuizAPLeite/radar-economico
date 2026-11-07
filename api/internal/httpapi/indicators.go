package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// Store é tudo o que os handlers precisam do banco. Uma interface pode
// "embutir" outras: Store tem todos os métodos de IndicatorStore e de
// FuelStore (ver fuels.go). O *db.Queries gerado pelo sqlc já tem esses
// métodos, então satisfaz a interface sem declarar nada; nos testes, uma
// versão falsa ocupa o lugar dele.
type Store interface {
	IndicatorStore
	FuelStore
}

// IndicatorStore é o que os handlers de indicadores precisam do banco.
type IndicatorStore interface {
	ListIndicators(ctx context.Context) ([]db.ListIndicatorsRow, error)
	GetIndicator(ctx context.Context, id string) (db.MartIndicator, error)
	ListIndicatorValues(ctx context.Context, arg db.ListIndicatorValuesParams) ([]db.ListIndicatorValuesRow, error)
}

type handlers struct {
	store  Store
	logger *slog.Logger
}

// --- Formatos das respostas ---------------------------------------------------

// As tags `json:"..."` dizem ao encoding/json o nome de cada campo no JSON.
type indicatorJSON struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Unit       string    `json:"unit"`
	Frequency  string    `json:"frequency"`
	Source     string    `json:"source"`
	SourceCode string    `json:"source_code"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Ponteiro + omitempty: quando é nil, o campo nem aparece no JSON.
	// Só a listagem preenche o valor mais recente.
	Latest *pointJSON `json:"latest,omitempty"`
}

type pointJSON struct {
	Date  string  `json:"date"` // AAAA-MM-DD
	Value float64 `json:"value"`
}

type listResponse struct {
	Data []indicatorJSON `json:"data"`
}

type valuesResponse struct {
	Indicator indicatorJSON `json:"indicator"`
	Data      []pointJSON   `json:"data"`
}

// indicatorFromModel converte o struct do banco no formato da API.
// source_run_id fica de fora: é um detalhe interno do pipeline.
func indicatorFromModel(m db.MartIndicator) indicatorJSON {
	return indicatorJSON{
		ID:         m.ID,
		Name:       m.Name,
		Unit:       m.Unit,
		Frequency:  m.Frequency,
		Source:     m.Source,
		SourceCode: m.SourceCode,
		UpdatedAt:  m.UpdatedAt.UTC(), // sempre em UTC, independente do fuso do servidor
	}
}

func newPoint(date time.Time, value float64) pointJSON {
	return pointJSON{Date: date.Format(time.DateOnly), Value: value}
}

// --- Handlers -----------------------------------------------------------------

func (h *handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// listIndicators responde GET /v1/indicators com todos os indicadores e o
// valor mais recente de cada um.
func (h *handlers) listIndicators(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.ListIndicators(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	data := make([]indicatorJSON, 0, len(rows))
	for _, row := range rows {
		ind := indicatorFromModel(row.MartIndicator)
		latest := newPoint(row.LastDate, row.LastValue)
		ind.Latest = &latest
		data = append(data, ind)
	}
	writeJSON(w, http.StatusOK, listResponse{Data: data})
}

// listIndicatorValues responde GET /v1/indicators/{id}/values?from=&to=.
func (h *handlers) listIndicatorValues(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	from, to, err := parseDateRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}

	ind, err := h.store.GetIndicator(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("indicador %q não encontrado", id))
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	rows, err := h.store.ListIndicatorValues(r.Context(), db.ListIndicatorValuesParams{
		IndicatorID: id,
		FromDate:    from,
		ToDate:      to,
	})
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	// make([]T, 0, n) cria uma lista VAZIA com espaço reservado para n itens.
	// Diferente de uma lista nil, ela vira [] no JSON, e não null: quem
	// consome a API pode sempre iterar sobre "data" sem checar nulo.
	data := make([]pointJSON, 0, len(rows))
	for _, row := range rows {
		data = append(data, newPoint(row.RefDate, row.Value))
	}
	writeJSON(w, http.StatusOK, valuesResponse{Indicator: indicatorFromModel(ind), Data: data})
}

// internalError registra o erro real e responde 500 com uma mensagem
// genérica: detalhes internos (SQL, nomes de tabela) não vazam para o cliente.
func (h *handlers) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("erro interno",
		"path", r.URL.Path,
		"request_id", middleware.GetReqID(r.Context()),
		"err", err,
	)
	writeError(w, http.StatusInternalServerError, "internal", "erro interno; tente novamente")
}

// --- Parâmetros ---------------------------------------------------------------

// Sem from/to, o intervalo cobre qualquer data que possa existir na mart.
var (
	minDate = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	maxDate = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
)

// parseDateRange lê ?from=AAAA-MM-DD&to=AAAA-MM-DD (ambos opcionais e inclusivos).
func parseDateRange(q url.Values) (from, to time.Time, err error) {
	from, to = minDate, maxDate
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse(time.DateOnly, v); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parâmetro from inválido (%q): use AAAA-MM-DD", v)
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse(time.DateOnly, v); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parâmetro to inválido (%q): use AAAA-MM-DD", v)
		}
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("from (%s) é depois de to (%s)",
			from.Format(time.DateOnly), to.Format(time.DateOnly))
	}
	return from, to, nil
}
