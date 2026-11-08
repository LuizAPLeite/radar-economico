package collect

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LuizAPLeite/radar-economico/collector/internal/sgs"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestBuildRequests_FullHistory(t *testing.T) {
	series := []Series{
		{Code: 1, Daily: true},
		{Code: 433, Daily: false},
	}
	reqs := BuildRequests(series, nil, date(2000, 1, 1), date(2026, 11, 6))

	// 3 janelas para a diária (2000-2009, 2010-2019, 2020-2026) + 1 para a mensal.
	if len(reqs) != 4 {
		t.Fatalf("len = %d, quer 4: %+v", len(reqs), reqs)
	}
	for _, r := range reqs[:3] {
		if r.SeriesCode != 1 || r.Start.IsZero() || r.End.IsZero() {
			t.Errorf("consulta diária inválida: %+v", r)
		}
	}
	if m := reqs[3]; m.SeriesCode != 433 || !m.Start.IsZero() || !m.End.IsZero() {
		t.Errorf("consulta mensal deveria vir sem datas: %+v", m)
	}
}

func TestBuildRequests_Incremental(t *testing.T) {
	series := []Series{
		{Code: 1, Daily: true},
		{Code: 433, Daily: false},
		{Code: 432, Daily: true}, // nunca coletada: histórico completo
	}
	since := map[int]time.Time{1: date(2026, 10, 7), 433: date(2026, 7, 9)}
	reqs := BuildRequests(series, since, date(2000, 1, 1), date(2026, 11, 6))

	// Uma consulta curta para cada série já coletada (a mensal também, com
	// datas) + 3 janelas de 10 anos para a que nunca foi coletada.
	if len(reqs) != 5 {
		t.Fatalf("len = %d, quer 5: %+v", len(reqs), reqs)
	}
	want := []struct {
		code       int
		start, end time.Time
	}{
		{1, date(2026, 10, 7), date(2026, 11, 6)},
		{433, date(2026, 7, 9), date(2026, 11, 6)},
	}
	for i, w := range want {
		r := reqs[i]
		if r.SeriesCode != w.code || !r.Start.Equal(w.start) || !r.End.Equal(w.end) {
			t.Errorf("consulta %d = %+v, quer série %d de %s a %s", i, r, w.code,
				w.start.Format(time.DateOnly), w.end.Format(time.DateOnly))
		}
	}
	if r := reqs[2]; r.SeriesCode != 432 || !r.Start.Equal(date(2000, 1, 1)) {
		t.Errorf("série nunca coletada deveria começar em 2000: %+v", r)
	}
}

func TestIncrementalStarts(t *testing.T) {
	series := []Series{
		{Code: 1, Daily: true},
		{Code: 433, Daily: false},
		{Code: 432, Daily: true},
	}
	coverage := map[int]time.Time{
		1:    date(2026, 11, 6),
		433:  date(2026, 11, 6),
		9999: date(2026, 11, 6), // série que não está na lista: ignorada
	}

	got := IncrementalStarts(series, coverage, date(2000, 1, 1))

	want := map[int]time.Time{
		1:   date(2026, 10, 7), // 30 dias antes
		433: date(2026, 7, 9),  // 120 dias antes
	}
	if len(got) != len(want) {
		t.Fatalf("got = %v, quer %v (a 432 nunca foi coletada e fica de fora)", got, want)
	}
	for code, w := range want {
		if !got[code].Equal(w) {
			t.Errorf("série %d começa em %s, quer %s", code, got[code].Format(time.DateOnly), w.Format(time.DateOnly))
		}
	}
}

func TestIncrementalStarts_NeverBeforeHistoryStart(t *testing.T) {
	got := IncrementalStarts([]Series{{Code: 1, Daily: true}},
		map[int]time.Time{1: date(2000, 1, 15)}, date(2000, 1, 1))
	if !got[1].Equal(date(2000, 1, 1)) {
		t.Errorf("começa em %s, quer 2000-01-01 (o início do histórico)", got[1].Format(time.DateOnly))
	}
}

// fakeFetcher simula a API: falha nas séries listadas em failCodes e
// registra quantas consultas rodaram ao mesmo tempo.
type fakeFetcher struct {
	failCodes map[int]bool
	inFlight  atomic.Int32
	maxSeen   atomic.Int32
}

func (f *fakeFetcher) Fetch(_ context.Context, req sgs.Request) (sgs.Response, error) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for { // guarda o maior valor visto (compare-and-swap)
		old := f.maxSeen.Load()
		if n <= old || f.maxSeen.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond) // dá tempo para as goroutines se sobreporem

	if f.failCodes[req.SeriesCode] {
		return sgs.Response{}, errors.New("falha simulada")
	}
	status := http.StatusOK
	if req.SeriesCode == 404 {
		status = http.StatusNotFound
	}
	return sgs.Response{Request: req, StatusCode: status, Body: []byte(`[]`)}, nil
}

type fakeSaver struct {
	mu    sync.Mutex
	saved []sgs.Response
}

func (s *fakeSaver) SaveResponse(_ context.Context, _ int64, resp sgs.Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, resp)
	return nil
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRun_SavesAllAndRespectsConcurrency(t *testing.T) {
	var reqs []sgs.Request
	for i := range 20 {
		reqs = append(reqs, sgs.Request{SeriesCode: 1000 + i})
	}
	f := &fakeFetcher{}
	s := &fakeSaver{}

	stats, err := BCB(context.Background(), discard, f, s, 1, reqs, 4)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if stats.Saved != 20 || len(s.saved) != 20 {
		t.Errorf("gravadas = %d (stats %d), quer 20", len(s.saved), stats.Saved)
	}
	if got := f.maxSeen.Load(); got > 4 {
		t.Errorf("máximo simultâneo = %d, limite era 4", got)
	}
	if got := f.maxSeen.Load(); got < 2 {
		t.Errorf("máximo simultâneo = %d: as consultas não rodaram em paralelo", got)
	}
}

func TestRun_FailureDoesNotStopOthers(t *testing.T) {
	reqs := []sgs.Request{{SeriesCode: 1}, {SeriesCode: 2}, {SeriesCode: 404}, {SeriesCode: 3}}
	f := &fakeFetcher{failCodes: map[int]bool{2: true}}
	s := &fakeSaver{}

	stats, err := BCB(context.Background(), discard, f, s, 1, reqs, 2)

	if err == nil {
		t.Fatal("esperava erro da série 2")
	}
	want := BCBStats{Saved: 3, Empty: 1, Failed: 1}
	if stats != want {
		t.Errorf("stats = %+v, quer %+v", stats, want)
	}
	if len(s.saved) != 3 {
		t.Errorf("gravadas = %d, quer 3", len(s.saved))
	}
}
