package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
