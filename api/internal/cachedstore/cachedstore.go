// Package cachedstore guarda em memória o que a API lê da mart.
//
// Store é um decorator: implementa a mesma interface httpapi.Store que o
// acesso ao banco (db.Queries) e embrulha esse acesso. Os handlers não sabem
// se falam com o banco ou com o cache; quem escolhe é o main.go.
//
// Cada consulta fica guardada pela sua chave (o indicador; o combustível e o
// recorte; o combustível e o mês) durante o TTL. Séries com período
// (from/to) são guardadas inteiras, e o período é recortado em memória:
// pedidos de períodos diferentes da mesma série saem da mesma entrada. Erros
// nunca são guardados: a próxima requisição tenta de novo.
package cachedstore

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/LuizAPLeite/radar-economico/api/internal/cache"
	"github.com/LuizAPLeite/radar-economico/api/internal/db"
	"github.com/LuizAPLeite/radar-economico/api/internal/httpapi"
)

// Store guarda em memória as respostas de outro httpapi.Store.
type Store struct {
	next httpapi.Store

	// Um cache por tipo de valor: um Cache[V] guarda um tipo só, e assim o
	// compilador confere cada uso, sem conversões de tipo.
	indicators *cache.Cache[[]db.ListIndicatorsRow]
	indicator  *cache.Cache[db.MartIndicator]
	values     *cache.Cache[[]db.ListIndicatorValuesRow]
	fuels      *cache.Cache[[]db.ListFuelProductsRow]
	fuel       *cache.Cache[db.MartFuelProduct]
	weekly     *cache.Cache[[]db.ListFuelPricesWeeklyRow]
	months     *cache.Cache[[]time.Time]
	municipal  *cache.Cache[[]db.ListFuelPricesMunicipalRow]
	regimes    *cache.Cache[[]db.MartAnalysisRegime]
	lags       *cache.Cache[[]db.ListLagCorrelationsRow]
	rolling    *cache.Cache[[]db.ListRollingCorrelationsRow]
	forecasts  *cache.Cache[[]db.ListFuelForecastsRow]
	backtests  *cache.Cache[[]db.ListForecastBacktestsRow]
	official   *cache.Cache[[]db.ListOfficialFuelPricesRow]
	freshness  *cache.Cache[[]db.ListDatasetFreshnessRow]
	lastRuns   *cache.Cache[[]db.ListLastRunsRow]
	lastOK     *cache.Cache[[]db.ListLastSuccessfulRunsRow]
}

// Conferência em tempo de compilação: se Store deixar de implementar
// httpapi.Store (um método novo na interface, por exemplo), o build quebra
// aqui, e não só quando o main.go tentar usá-lo.
var _ httpapi.Store = (*Store)(nil)

// New embrulha next; cada resposta guardada vale por ttl.
func New(next httpapi.Store, ttl time.Duration) *Store {
	return &Store{
		next:       next,
		indicators: cache.New[[]db.ListIndicatorsRow](ttl),
		indicator:  cache.New[db.MartIndicator](ttl),
		values:     cache.New[[]db.ListIndicatorValuesRow](ttl),
		fuels:      cache.New[[]db.ListFuelProductsRow](ttl),
		fuel:       cache.New[db.MartFuelProduct](ttl),
		weekly:     cache.New[[]db.ListFuelPricesWeeklyRow](ttl),
		months:     cache.New[[]time.Time](ttl),
		municipal:  cache.New[[]db.ListFuelPricesMunicipalRow](ttl),
		regimes:    cache.New[[]db.MartAnalysisRegime](ttl),
		lags:       cache.New[[]db.ListLagCorrelationsRow](ttl),
		rolling:    cache.New[[]db.ListRollingCorrelationsRow](ttl),
		forecasts:  cache.New[[]db.ListFuelForecastsRow](ttl),
		backtests:  cache.New[[]db.ListForecastBacktestsRow](ttl),
		official:   cache.New[[]db.ListOfficialFuelPricesRow](ttl),
		freshness:  cache.New[[]db.ListDatasetFreshnessRow](ttl),
		lastRuns:   cache.New[[]db.ListLastRunsRow](ttl),
		lastOK:     cache.New[[]db.ListLastSuccessfulRunsRow](ttl),
	}
}

// --- Indicadores ----------------------------------------------------------------

func (s *Store) ListIndicators(ctx context.Context) ([]db.ListIndicatorsRow, error) {
	return cachedRows(s.indicators, "", func() ([]db.ListIndicatorsRow, error) {
		return s.next.ListIndicators(ctx)
	})
}

func (s *Store) GetIndicator(ctx context.Context, id string) (db.MartIndicator, error) {
	return cached(s.indicator, id, func() (db.MartIndicator, error) {
		return s.next.GetIndicator(ctx, id)
	})
}

func (s *Store) ListIndicatorValues(ctx context.Context, arg db.ListIndicatorValuesParams) ([]db.ListIndicatorValuesRow, error) {
	all, err := cached(s.values, arg.IndicatorID, func() ([]db.ListIndicatorValuesRow, error) {
		return s.next.ListIndicatorValues(ctx, db.ListIndicatorValuesParams{
			IndicatorID: arg.IndicatorID, FromDate: httpapi.MinDate, ToDate: httpapi.MaxDate,
		})
	})
	if err != nil {
		return nil, err
	}
	return between(all, arg.FromDate, arg.ToDate, func(r db.ListIndicatorValuesRow) time.Time { return r.RefDate }), nil
}

// --- Combustíveis ---------------------------------------------------------------

func (s *Store) ListFuelProducts(ctx context.Context) ([]db.ListFuelProductsRow, error) {
	return cachedRows(s.fuels, "", func() ([]db.ListFuelProductsRow, error) {
		return s.next.ListFuelProducts(ctx)
	})
}

func (s *Store) GetFuelProduct(ctx context.Context, id string) (db.MartFuelProduct, error) {
	return cached(s.fuel, id, func() (db.MartFuelProduct, error) {
		return s.next.GetFuelProduct(ctx, id)
	})
}

func (s *Store) ListFuelPricesWeekly(ctx context.Context, arg db.ListFuelPricesWeeklyParams) ([]db.ListFuelPricesWeeklyRow, error) {
	key := arg.ProductID + "|" + arg.Scope
	all, err := cached(s.weekly, key, func() ([]db.ListFuelPricesWeeklyRow, error) {
		return s.next.ListFuelPricesWeekly(ctx, db.ListFuelPricesWeeklyParams{
			ProductID: arg.ProductID, Scope: arg.Scope, FromDate: httpapi.MinDate, ToDate: httpapi.MaxDate,
		})
	})
	if err != nil {
		return nil, err
	}
	return between(all, arg.FromDate, arg.ToDate, func(r db.ListFuelPricesWeeklyRow) time.Time { return r.WeekStart }), nil
}

func (s *Store) ListFuelMonths(ctx context.Context, productID string) ([]time.Time, error) {
	return cachedRows(s.months, productID, func() ([]time.Time, error) {
		return s.next.ListFuelMonths(ctx, productID)
	})
}

func (s *Store) ListFuelPricesMunicipal(ctx context.Context, arg db.ListFuelPricesMunicipalParams) ([]db.ListFuelPricesMunicipalRow, error) {
	key := arg.ProductID + "|" + arg.Month.Format("2006-01")
	return cachedRows(s.municipal, key, func() ([]db.ListFuelPricesMunicipalRow, error) {
		return s.next.ListFuelPricesMunicipal(ctx, arg)
	})
}

// --- Análises -------------------------------------------------------------------

func (s *Store) ListAnalysisRegimes(ctx context.Context) ([]db.MartAnalysisRegime, error) {
	return cachedRows(s.regimes, "", func() ([]db.MartAnalysisRegime, error) {
		return s.next.ListAnalysisRegimes(ctx)
	})
}

func (s *Store) ListLagCorrelations(ctx context.Context, productID string) ([]db.ListLagCorrelationsRow, error) {
	return cachedRows(s.lags, productID, func() ([]db.ListLagCorrelationsRow, error) {
		return s.next.ListLagCorrelations(ctx, productID)
	})
}

func (s *Store) ListRollingCorrelations(ctx context.Context, productID string) ([]db.ListRollingCorrelationsRow, error) {
	return cachedRows(s.rolling, productID, func() ([]db.ListRollingCorrelationsRow, error) {
		return s.next.ListRollingCorrelations(ctx, productID)
	})
}

func (s *Store) ListFuelForecasts(ctx context.Context, productID string) ([]db.ListFuelForecastsRow, error) {
	return cachedRows(s.forecasts, productID, func() ([]db.ListFuelForecastsRow, error) {
		return s.next.ListFuelForecasts(ctx, productID)
	})
}

func (s *Store) ListForecastBacktests(ctx context.Context, productID string) ([]db.ListForecastBacktestsRow, error) {
	return cachedRows(s.backtests, productID, func() ([]db.ListForecastBacktestsRow, error) {
		return s.next.ListForecastBacktests(ctx, productID)
	})
}

func (s *Store) ListOfficialFuelPrices(ctx context.Context, arg db.ListOfficialFuelPricesParams) ([]db.ListOfficialFuelPricesRow, error) {
	all, err := cached(s.official, arg.ProductID, func() ([]db.ListOfficialFuelPricesRow, error) {
		return s.next.ListOfficialFuelPrices(ctx, db.ListOfficialFuelPricesParams{
			ProductID: arg.ProductID, FromDate: httpapi.MinDate, ToDate: httpapi.MaxDate,
		})
	})
	if err != nil {
		return nil, err
	}
	return between(all, arg.FromDate, arg.ToDate, func(r db.ListOfficialFuelPricesRow) time.Time { return r.WeekStart }), nil
}

// --- Saúde dos dados ------------------------------------------------------------
//
// Também passam pelo cache: um monitor consultando /health/data a cada poucos
// minutos não pode manter o banco acordado (o Neon cobra o tempo ligado). O
// "agora" é calculado a cada requisição, então a idade dos dados é sempre a
// atual; só a leitura do banco pode ter até um TTL.

func (s *Store) ListDatasetFreshness(ctx context.Context) ([]db.ListDatasetFreshnessRow, error) {
	return cachedRows(s.freshness, "", func() ([]db.ListDatasetFreshnessRow, error) {
		return s.next.ListDatasetFreshness(ctx)
	})
}

func (s *Store) ListLastRuns(ctx context.Context) ([]db.ListLastRunsRow, error) {
	return cachedRows(s.lastRuns, "", func() ([]db.ListLastRunsRow, error) {
		return s.next.ListLastRuns(ctx)
	})
}

func (s *Store) ListLastSuccessfulRuns(ctx context.Context) ([]db.ListLastSuccessfulRunsRow, error) {
	return cachedRows(s.lastOK, "", func() ([]db.ListLastSuccessfulRunsRow, error) {
		return s.next.ListLastSuccessfulRuns(ctx)
	})
}

// --- Funções auxiliares ---------------------------------------------------------

// cached devolve o valor guardado em key; se não houver (ou se já tiver
// vencido), chama load e guarda o resultado. Erros não são guardados.
//
// É uma função, e não um método de Store, porque no Go um método não pode
// ter parâmetros de tipo próprios ([V any]); só o tipo dele pode.
func cached[V any](c *cache.Cache[V], key string, load func() (V, error)) (V, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	v, err := load()
	if err != nil {
		return v, err
	}
	c.Set(key, v)
	return v, nil
}

// cachedRows é o cached para listas, devolvendo uma cópia: o slice guardado
// é compartilhado por todas as requisições, e quem o recebe poderia alterá-lo.
func cachedRows[R any](c *cache.Cache[[]R], key string, load func() ([]R, error)) ([]R, error) {
	rows, err := cached(c, key, load)
	if err != nil {
		return nil, err
	}
	return slices.Clone(rows), nil
}

// between devolve uma cópia das linhas com data entre from e to (inclusive).
// As linhas vêm em ordem de data (as queries têm ORDER BY), então a busca
// binária (sort.Search) acha as duas pontas sem percorrer a série inteira.
func between[R any](rows []R, from, to time.Time, date func(R) time.Time) []R {
	start := sort.Search(len(rows), func(i int) bool { return !date(rows[i]).Before(from) })
	end := sort.Search(len(rows), func(i int) bool { return date(rows[i]).After(to) })
	if start >= end {
		return nil
	}
	return slices.Clone(rows[start:end])
}
