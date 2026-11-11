package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LuizAPLeite/radar-economico/collector/internal/anp"
	"github.com/LuizAPLeite/radar-economico/collector/internal/sgs"
)

// Teste de integração: precisa de um Postgres com as migrations aplicadas.
// Sem TEST_DATABASE_URL ele é pulado, para `go test` funcionar em qualquer lugar.
//
//	TEST_DATABASE_URL=postgres://radar:radar@localhost:5432/radar?sslmode=disable go test ./collector/internal/store
func newTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definida; pulando teste de integração")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return New(pool), pool
}

func TestStore_SaveIBGESkipsUnchangedContent(t *testing.T) {
	s, pool := newTestStore(t)
	ctx := context.Background()

	runID, err := s.StartRun(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM raw.ibge_responses WHERE run_id = $1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM raw.collection_runs WHERE id = $1`, runID)
	})

	steps := []struct {
		name, sha string
		wantSaved bool
	}{
		{"conteúdo novo é gravado", "teste-hash-a", true},
		{"o mesmo conteúdo de novo é ignorado", "teste-hash-a", false},
		{"conteúdo diferente é gravado", "teste-hash-b", true},
	}
	for _, st := range steps {
		saved, err := s.SaveIBGE(ctx, runID, "municipios", "http://ibge/m", st.sha, []byte(`[{"id": 1}]`))
		if err != nil {
			t.Fatal(err)
		}
		if saved != st.wantSaved {
			t.Errorf("%s: saved = %v, quer %v", st.name, saved, st.wantSaved)
		}
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM raw.ibge_responses WHERE run_id = $1`, runID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("linhas gravadas = %d, quer 2", n)
	}
}

func TestStore_SaveSourceFileSkipsUnchangedContent(t *testing.T) {
	s, pool := newTestStore(t)
	ctx := context.Background()

	runID, err := s.StartRun(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM raw.source_files WHERE run_id = $1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM raw.collection_runs WHERE id = $1`, runID)
	})

	steps := []struct {
		name, file, sha string
		wantSaved       bool
	}{
		{"arquivo novo é gravado", "teste-a", "hash-1", true},
		{"o mesmo conteúdo de novo é ignorado", "teste-a", "hash-1", false},
		{"o mesmo conteúdo em outro arquivo é gravado", "teste-b", "hash-1", true},
		{"conteúdo diferente é gravado", "teste-a", "hash-2", true},
	}
	for _, st := range steps {
		saved, err := s.SaveSourceFile(ctx, runID, "teste", st.file, "http://x/"+st.file, st.sha, 3, []byte{1, 2, 3})
		if err != nil {
			t.Fatal(err)
		}
		if saved != st.wantSaved {
			t.Errorf("%s: saved = %v, quer %v", st.name, saved, st.wantSaved)
		}
	}
}

func TestStore_ANPFiles(t *testing.T) {
	s, pool := newTestStore(t)
	ctx := context.Background()

	runID, err := s.StartRun(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM raw.anp_files WHERE run_id = $1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM raw.collection_runs WHERE id = $1`, runID)
	})

	// Um mês bem no futuro, para não se misturar com dados reais do banco de desenvolvimento.
	f := anp.File{Group: anp.GroupDieselGNV, Month: time.Date(2099, time.March, 1, 0, 0, 0, 0, time.UTC), URL: "http://anp/x.csv"}
	for _, sha := range []string{"versao-1", "versao-2"} {
		if err := s.SaveANPFile(ctx, runID, f, sha, 10, []byte{0x1f, 0x8b}); err != nil {
			t.Fatal(err)
		}
	}

	latest, err := s.LatestANPHashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A chave lida do banco tem que bater com a montada no Go (mês como texto).
	if got := latest[f.Key()]; got != "versao-2" {
		t.Errorf("hash mais recente de %+v = %q, quer \"versao-2\"", f.Key(), got)
	}
}

func TestStore_BCBCoverage(t *testing.T) {
	s, pool := newTestStore(t)
	ctx := context.Background()

	// Códigos de série que não existem no SGS, para não se misturar com os
	// dados reais do banco de desenvolvimento.
	const daily, monthly = 90001, 90002
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	save := func(runErr error, responses ...sgs.Response) {
		t.Helper()
		runID, err := s.StartRun(ctx, "test")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM raw.bcb_sgs_responses WHERE run_id = $1`, runID)
			_, _ = pool.Exec(ctx, `DELETE FROM raw.collection_runs WHERE id = $1`, runID)
		})
		for _, r := range responses {
			if err := s.SaveResponse(ctx, runID, r); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.FinishRun(ctx, runID, runErr); err != nil {
			t.Fatal(err)
		}
	}
	window := func(code int, start, end time.Time) sgs.Response {
		return sgs.Response{
			Request: sgs.Request{SeriesCode: code, Start: start, End: end},
			URL:     "http://exemplo", StatusCode: 200, Body: []byte(`[]`),
		}
	}

	save(nil,
		window(daily, day(2016, 11, 7), day(2026, 11, 6)),
		sgs.Response{Request: sgs.Request{SeriesCode: monthly}, URL: "http://exemplo", StatusCode: 200, Body: []byte(`[]`)},
	)
	// Uma coleta que falhou não conta, mesmo indo mais longe.
	save(errors.New("falha de teste"), window(daily, day(2099, 1, 1), day(2099, 12, 31)))

	coverage, err := s.BCBCoverage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := coverage[daily]; !got.Equal(day(2026, 11, 6)) {
		t.Errorf("série diária coberta até %s, quer 2026-11-06 (o fim da janela)", got.Format(time.DateOnly))
	}
	// Consulta sem datas: cobre até o dia da coleta, que foi hoje (em Brasília).
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	y, m, d := time.Now().In(loc).Date()
	if got := coverage[monthly]; !got.Equal(day(y, m, d)) {
		t.Errorf("série mensal coberta até %s, quer hoje", got.Format(time.DateOnly))
	}
}

func TestStore_RunLifecycle(t *testing.T) {
	s, pool := newTestStore(t)
	ctx := context.Background()

	runID, err := s.StartRun(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	// Apaga o que o teste criou, mesmo se ele falhar no meio.
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM raw.bcb_sgs_responses WHERE run_id = $1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM raw.collection_runs WHERE id = $1`, runID)
	})

	withDates := sgs.Response{
		Request: sgs.Request{
			SeriesCode: 1,
			Start:      time.Date(2016, 11, 7, 0, 0, 0, 0, time.UTC),
			End:        time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC),
		},
		URL: "http://exemplo/1", StatusCode: 200,
		Body: []byte(`[{"data":"06/11/2026","valor":"5.5013"}]`),
	}
	noDates := sgs.Response{
		Request: sgs.Request{SeriesCode: 433},
		URL:     "http://exemplo/433", StatusCode: 404,
		Body: []byte(`{"erro":{"detail":"Value(s) not found"}}`),
	}
	for _, r := range []sgs.Response{withDates, noDates} {
		if err := s.SaveResponse(ctx, runID, r); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.FinishRun(ctx, runID, errors.New("falha de teste")); err != nil {
		t.Fatal(err)
	}

	var (
		status, errMsg string
		finished       *time.Time
	)
	err = pool.QueryRow(ctx,
		`SELECT status, error, finished_at FROM raw.collection_runs WHERE id = $1`, runID,
	).Scan(&status, &errMsg, &finished)
	if err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errMsg != "falha de teste" || finished == nil {
		t.Errorf("run = (%s, %q, %v)", status, errMsg, finished)
	}

	var (
		start, end *time.Time
		valor      string
	)
	err = pool.QueryRow(ctx,
		`SELECT window_start, window_end, payload->0->>'valor'
		   FROM raw.bcb_sgs_responses WHERE run_id = $1 AND series_code = 1`, runID,
	).Scan(&start, &end, &valor)
	if err != nil {
		t.Fatal(err)
	}
	if start == nil || start.Format(time.DateOnly) != "2016-11-07" || end.Format(time.DateOnly) != "2026-11-06" {
		t.Errorf("janela = %v → %v", start, end)
	}
	if valor != "5.5013" {
		t.Errorf("valor no payload = %q", valor)
	}

	var nullStart bool
	err = pool.QueryRow(ctx,
		`SELECT window_start IS NULL FROM raw.bcb_sgs_responses WHERE run_id = $1 AND series_code = 433`, runID,
	).Scan(&nullStart)
	if err != nil {
		t.Fatal(err)
	}
	if !nullStart {
		t.Error("consulta sem datas deveria gravar window_start NULL")
	}
}
