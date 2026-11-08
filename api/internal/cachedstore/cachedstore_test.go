package cachedstore

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
	"github.com/LuizAPLeite/radar-economico/api/internal/httpapi"
)

// countingStore é um httpapi.Store falso que conta as chamadas a cada método,
// para os testes saberem quando o cache foi ao "banco".
type countingStore struct {
	mu        sync.Mutex
	calls     map[string]int
	values    []db.ListIndicatorValuesRow // em ordem de data, como a query devolve
	gotValues db.ListIndicatorValuesParams
	failNext  int // quantas chamadas a GetIndicator ainda falham
}

func newCountingStore() *countingStore {
	return &countingStore{calls: map[string]int{}}
}

func (f *countingStore) count(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[method]++
}

func (f *countingStore) callsTo(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *countingStore) ListIndicators(context.Context) ([]db.ListIndicatorsRow, error) {
	f.count("ListIndicators")
	return []db.ListIndicatorsRow{{MartIndicator: db.MartIndicator{ID: "selic"}}}, nil
}

func (f *countingStore) GetIndicator(_ context.Context, id string) (db.MartIndicator, error) {
	f.count("GetIndicator")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext > 0 {
		f.failNext--
		return db.MartIndicator{}, errors.New("conexão recusada")
	}
	if id != "selic" {
		return db.MartIndicator{}, pgx.ErrNoRows
	}
	return db.MartIndicator{ID: id, Name: "Meta Selic"}, nil
}

func (f *countingStore) ListIndicatorValues(_ context.Context, p db.ListIndicatorValuesParams) ([]db.ListIndicatorValuesRow, error) {
	f.count("ListIndicatorValues")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gotValues = p
	return f.values, nil
}

func (f *countingStore) ListFuelProducts(context.Context) ([]db.ListFuelProductsRow, error) {
	f.count("ListFuelProducts")
	return nil, nil
}

func (f *countingStore) GetFuelProduct(_ context.Context, id string) (db.MartFuelProduct, error) {
	f.count("GetFuelProduct")
	return db.MartFuelProduct{ID: id}, nil
}

func (f *countingStore) ListFuelPricesWeekly(_ context.Context, p db.ListFuelPricesWeeklyParams) ([]db.ListFuelPricesWeeklyRow, error) {
	f.count("ListFuelPricesWeekly")
	return []db.ListFuelPricesWeeklyRow{{WeekStart: day(2026, 10, 26), AvgPrice: 6.505}}, nil
}

func (f *countingStore) ListFuelMonths(context.Context, string) ([]time.Time, error) {
	f.count("ListFuelMonths")
	return []time.Time{day(2026, 10, 1)}, nil
}

func (f *countingStore) ListFuelPricesMunicipal(_ context.Context, p db.ListFuelPricesMunicipalParams) ([]db.ListFuelPricesMunicipalRow, error) {
	f.count("ListFuelPricesMunicipal")
	return []db.ListFuelPricesMunicipalRow{{Name: "Barreiras", AvgPrice: 6.89}}, nil
}

func (f *countingStore) ListDatasetFreshness(context.Context) ([]db.ListDatasetFreshnessRow, error) {
	f.count("ListDatasetFreshness")
	return []db.ListDatasetFreshnessRow{{Dataset: "selic", Latest: day(2026, 11, 8)}}, nil
}

func (f *countingStore) ListLastRuns(context.Context) ([]db.ListLastRunsRow, error) {
	f.count("ListLastRuns")
	return nil, nil
}

func (f *countingStore) ListLastSuccessfulRuns(context.Context) ([]db.ListLastSuccessfulRunsRow, error) {
	f.count("ListLastSuccessfulRuns")
	return nil, nil
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

var ctx = context.Background()

func TestRepeatedCallsGoToTheStoreOnce(t *testing.T) {
	inner := newCountingStore()
	s := New(inner, time.Minute)

	for range 3 {
		if _, err := s.ListIndicators(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetFuelProduct(ctx, "gasolina"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ListFuelMonths(ctx, "gasolina"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ListDatasetFreshness(ctx); err != nil {
			t.Fatal(err)
		}
	}

	for _, method := range []string{"ListIndicators", "GetFuelProduct", "ListFuelMonths", "ListDatasetFreshness"} {
		if n := inner.callsTo(method); n != 1 {
			t.Errorf("%s chamado %d vezes, quer 1", method, n)
		}
	}
}

func TestKeysIncludeTheParameters(t *testing.T) {
	inner := newCountingStore()
	s := New(inner, time.Minute)

	weekly := func(scope string) {
		t.Helper()
		_, err := s.ListFuelPricesWeekly(ctx, db.ListFuelPricesWeeklyParams{
			ProductID: "gasolina", Scope: scope, FromDate: httpapi.MinDate, ToDate: httpapi.MaxDate,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	weekly("BR")
	weekly("SP")
	weekly("SP")

	municipal := func(month time.Time) {
		t.Helper()
		if _, err := s.ListFuelPricesMunicipal(ctx, db.ListFuelPricesMunicipalParams{ProductID: "gasolina", Month: month}); err != nil {
			t.Fatal(err)
		}
	}
	municipal(day(2026, 9, 1))
	municipal(day(2026, 10, 1))
	municipal(day(2026, 10, 1))

	if n := inner.callsTo("ListFuelPricesWeekly"); n != 2 {
		t.Errorf("semanal: %d consultas, quer 2 (BR e SP)", n)
	}
	if n := inner.callsTo("ListFuelPricesMunicipal"); n != 2 {
		t.Errorf("municípios: %d consultas, quer 2 (set e out)", n)
	}
}

func TestPeriodsComeFromTheSameFullSeries(t *testing.T) {
	inner := newCountingStore()
	inner.values = []db.ListIndicatorValuesRow{
		{RefDate: day(2026, 11, 3), Value: 5.4935},
		{RefDate: day(2026, 11, 4), Value: 5.4778},
		{RefDate: day(2026, 11, 5), Value: 5.4621},
		{RefDate: day(2026, 11, 6), Value: 5.5013},
	}
	s := New(inner, time.Minute)

	values := func(from, to time.Time) []db.ListIndicatorValuesRow {
		t.Helper()
		rows, err := s.ListIndicatorValues(ctx, db.ListIndicatorValuesParams{IndicatorID: "usd_brl", FromDate: from, ToDate: to})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}

	// from e to são inclusivos.
	if got := values(day(2026, 11, 4), day(2026, 11, 5)); len(got) != 2 || got[0].Value != 5.4778 || got[1].Value != 5.4621 {
		t.Errorf("de 04 a 05/11 = %+v", got)
	}
	if got := values(httpapi.MinDate, httpapi.MaxDate); len(got) != 4 {
		t.Errorf("série inteira com %d valores, quer 4", len(got))
	}
	if got := values(day(2026, 12, 1), httpapi.MaxDate); len(got) != 0 {
		t.Errorf("depois do último valor = %+v, quer vazio", got)
	}

	// Três períodos, uma consulta só, e ela pediu a série inteira.
	if n := inner.callsTo("ListIndicatorValues"); n != 1 {
		t.Errorf("%d consultas, quer 1", n)
	}
	if p := inner.gotValues; !p.FromDate.Equal(httpapi.MinDate) || !p.ToDate.Equal(httpapi.MaxDate) {
		t.Errorf("consulta pediu %s a %s, quer a série inteira", p.FromDate, p.ToDate)
	}
}

func TestErrorsAreNotCached(t *testing.T) {
	inner := newCountingStore()
	inner.failNext = 1
	s := New(inner, time.Minute)

	if _, err := s.GetIndicator(ctx, "selic"); err == nil {
		t.Fatal("a primeira chamada deveria falhar")
	}
	got, err := s.GetIndicator(ctx, "selic")
	if err != nil || got.Name != "Meta Selic" {
		t.Fatalf("segunda chamada = (%+v, %v), quer a Selic", got, err)
	}

	// "Não encontrado" também é erro e não fica guardado: um id inventado na
	// URL não ocupa memória.
	for range 2 {
		if _, err := s.GetIndicator(ctx, "inventado"); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, quer pgx.ErrNoRows", err)
		}
	}
	if n := inner.callsTo("GetIndicator"); n != 4 {
		t.Errorf("%d consultas, quer 4 (nenhuma resposta com erro guardada)", n)
	}
}

func TestEntriesExpire(t *testing.T) {
	inner := newCountingStore()
	s := New(inner, time.Millisecond)

	if _, err := s.ListIndicators(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := s.ListIndicators(ctx); err != nil {
		t.Fatal(err)
	}

	if n := inner.callsTo("ListIndicators"); n != 2 {
		t.Errorf("%d consultas, quer 2 (a primeira resposta venceu)", n)
	}
}

func TestReturnedRowsAreCopies(t *testing.T) {
	s := New(newCountingStore(), time.Minute)

	first, _ := s.ListFuelPricesMunicipal(ctx, db.ListFuelPricesMunicipalParams{ProductID: "gasolina", Month: day(2026, 10, 1)})
	first[0].AvgPrice = 0 // um handler que alterasse a lista...

	second, _ := s.ListFuelPricesMunicipal(ctx, db.ListFuelPricesMunicipalParams{ProductID: "gasolina", Month: day(2026, 10, 1)})
	if second[0].AvgPrice != 6.89 { // ...não estragaria o que está guardado
		t.Errorf("AvgPrice = %v, quer 6.89", second[0].AvgPrice)
	}
}

// Muitas requisições ao mesmo tempo; com `go test -race`, o detector acusa
// qualquer acesso sem proteção.
func TestConcurrentUse(t *testing.T) {
	inner := newCountingStore()
	inner.values = []db.ListIndicatorValuesRow{{RefDate: day(2026, 11, 6), Value: 5.5013}}
	s := New(inner, time.Minute)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_, _ = s.ListIndicators(ctx)
			_, _ = s.ListIndicatorValues(ctx, db.ListIndicatorValuesParams{
				IndicatorID: "usd_brl", FromDate: httpapi.MinDate, ToDate: httpapi.MaxDate,
			})
		})
	}
	wg.Wait()
}
