package assistant

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// fakePlanner devolve sempre o mesmo plano (ou erro) e conta as chamadas.
type fakePlanner struct {
	plan  Plan
	err   error
	calls int
}

func (f *fakePlanner) Plan(context.Context, string) (Plan, error) {
	f.calls++
	return f.plan, f.err
}

// fakeExecutor devolve um resultado pronto e guarda o SQL recebido.
type fakeExecutor struct {
	result Result
	err    error
	gotSQL string
}

func (f *fakeExecutor) Run(_ context.Context, sql string) (Result, error) {
	f.gotSQL = sql
	return f.result, f.err
}

var brt = time.FixedZone("BRT", -3*60*60)

var selicPlan = Plan{
	Answerable: true, SQL: "SELECT ref_date AS dia, value AS selic FROM mart.indicator_values;",
	Title: "Selic", Explanation: "A meta Selic.",
	Chart: ChartSpec{Type: "line", X: "dia", Y: []string{"selic"}},
}

var selicResult = Result{
	Columns: []Column{{"dia", "date"}, {"selic", "number"}},
	Rows:    [][]any{{"2026-11-10", 13.75}, {"2026-11-11", 13.75}},
}

func newService(p Planner, e Executor, limit int) *Service {
	return NewService(p, e, slog.New(slog.NewTextHandler(io.Discard, nil)), limit, brt)
}

func TestAsk_AnswersAndCaches(t *testing.T) {
	p := &fakePlanner{plan: selicPlan}
	e := &fakeExecutor{result: selicResult}
	s := newService(p, e, 10)

	a, err := s.Ask(context.Background(), "  Qual a Selic?  ")
	if err != nil {
		t.Fatal(err)
	}
	if a.Question != "Qual a Selic?" || a.Cached || !a.Answerable || len(a.Rows) != 2 || a.Chart.Type != "line" {
		t.Errorf("resposta = %+v", a)
	}
	// O SQL executado é o limpo (sem o ";"), e é o que volta na resposta.
	if e.gotSQL != strings.TrimSuffix(selicPlan.SQL, ";") || a.SQL != e.gotSQL {
		t.Errorf("SQL executado = %q, devolvido = %q", e.gotSQL, a.SQL)
	}

	// A mesma pergunta escrita de outro jeito sai do cache, sem chamar o LLM.
	again, err := s.Ask(context.Background(), "qual a   SELIC")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Cached || again.Question != "qual a   SELIC" || p.calls != 1 {
		t.Errorf("cached = %v, chamadas ao LLM = %d", again.Cached, p.calls)
	}
}

func TestAsk_RejectsBadQuestions(t *testing.T) {
	s := newService(&fakePlanner{plan: selicPlan}, &fakeExecutor{}, 10)
	for _, q := range []string{"", "  ", "ab", strings.Repeat("é", MaxQuestionLength+1)} {
		if _, err := s.Ask(context.Background(), q); !errors.Is(err, ErrBadQuestion) {
			t.Errorf("pergunta de %d caracteres: erro = %v", len(q), err)
		}
	}
}

func TestAsk_DailyLimitResetsAtMidnightInBrasilia(t *testing.T) {
	p := &fakePlanner{plan: Plan{Answerable: false, Title: "Fora do assunto"}}
	s := newService(p, &fakeExecutor{}, 2)
	clock := time.Date(2026, 11, 12, 23, 0, 0, 0, brt)
	s.now = func() time.Time { return clock }

	ask := func(q string) error {
		_, err := s.Ask(context.Background(), q)
		return err
	}
	if ask("pergunta um") != nil || ask("pergunta dois") != nil {
		t.Fatal("as duas primeiras deveriam passar")
	}
	if err := ask("pergunta três"); !errors.Is(err, ErrDailyLimit) {
		t.Errorf("terceira: erro = %v, quer ErrDailyLimit", err)
	}
	// Do cache ainda dá para responder, mesmo sem limite.
	if err := ask("pergunta um"); err != nil {
		t.Errorf("pergunta repetida: %v", err)
	}
	// 23h + 2h = 01h do dia 13 em Brasília: o limite recomeça.
	clock = clock.Add(2 * time.Hour)
	if err := ask("pergunta três"); err != nil {
		t.Errorf("dia novo: %v", err)
	}
}

func TestAsk_UnanswerableHasNoSQLAndIsCached(t *testing.T) {
	p := &fakePlanner{plan: Plan{Answerable: false, SQL: "SELECT 1", Explanation: "Não temos dados de futebol."}}
	e := &fakeExecutor{}
	s := newService(p, e, 10)

	a, err := s.Ask(context.Background(), "Quem ganhou o jogo?")
	if err != nil {
		t.Fatal(err)
	}
	if a.Answerable || a.SQL != "" || e.gotSQL != "" || a.Explanation == "" {
		t.Errorf("resposta = %+v, SQL executado = %q", a, e.gotSQL)
	}
	if _, err := s.Ask(context.Background(), "Quem ganhou o jogo?"); err != nil || p.calls != 1 {
		t.Errorf("deveria vir do cache: %v, %d chamadas", err, p.calls)
	}
}

func TestAsk_BadPlansAreNotCached(t *testing.T) {
	unsafe := &fakePlanner{plan: Plan{Answerable: true, SQL: "DROP TABLE mart.indicators"}}
	e := &fakeExecutor{}
	s := newService(unsafe, e, 10)
	for range 2 {
		if _, err := s.Ask(context.Background(), "apague tudo"); !errors.Is(err, ErrBadPlan) || !errors.Is(err, ErrUnsafeSQL) {
			t.Errorf("erro = %v", err)
		}
	}
	if unsafe.calls != 2 || e.gotSQL != "" {
		t.Errorf("chamadas = %d (erro não entra no cache), SQL executado = %q", unsafe.calls, e.gotSQL)
	}

	failing := &fakeExecutor{err: errors.New(`column "x" does not exist`)}
	s = newService(&fakePlanner{plan: selicPlan}, failing, 10)
	if _, err := s.Ask(context.Background(), "Qual a Selic?"); !errors.Is(err, ErrBadPlan) {
		t.Errorf("erro do banco = %v, quer ErrBadPlan", err)
	}
}

func TestAsk_ProviderErrorsPassThrough(t *testing.T) {
	s := newService(&fakePlanner{err: ErrQuotaExceeded}, &fakeExecutor{}, 10)
	if _, err := s.Ask(context.Background(), "Qual a Selic?"); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("erro = %v", err)
	}
}

func TestCheckChart(t *testing.T) {
	cols := []Column{{"mes", "date"}, {"uf", "text"}, {"preco", "number"}, {"postos", "number"}}
	tests := []struct {
		name  string
		chart ChartSpec
		rows  int
		want  string
	}{
		{"linha no tempo", ChartSpec{"line", "mes", []string{"preco"}}, 12, "line"},
		{"barras por UF", ChartSpec{"bar", "uf", []string{"preco", "postos"}}, 27, "bar"},
		{"linha sem data no eixo X", ChartSpec{"line", "uf", []string{"preco"}}, 12, "none"},
		{"barras demais", ChartSpec{"bar", "uf", []string{"preco"}}, 31, "none"},
		{"coluna que não existe", ChartSpec{"line", "mes", []string{"valor"}}, 12, "none"},
		{"y de texto", ChartSpec{"bar", "mes", []string{"uf"}}, 12, "none"},
		{"y igual ao x", ChartSpec{"bar", "preco", []string{"preco"}}, 3, "none"},
		{"sem linhas", ChartSpec{"line", "mes", []string{"preco"}}, 0, "none"},
		{"tipo desconhecido", ChartSpec{"pie", "uf", []string{"preco"}}, 5, "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkChart(tt.chart, cols, tt.rows)
			if got.Type != tt.want {
				t.Errorf("tipo = %q, quer %q", got.Type, tt.want)
			}
			if got.Type == "none" && (got.X != "" || len(got.Y) != 0) {
				t.Errorf("none deveria vir sem eixos: %+v", got)
			}
		})
	}
}
