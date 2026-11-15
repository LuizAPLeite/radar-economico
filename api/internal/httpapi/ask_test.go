package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LuizAPLeite/radar-economico/api/internal/assistant"
	"github.com/LuizAPLeite/radar-economico/api/internal/ratelimit"
)

// fakeAsker devolve uma resposta pronta (ou erro) e guarda a pergunta.
type fakeAsker struct {
	answer assistant.Answer
	err    error
	got    string
}

func (f *fakeAsker) Ask(_ context.Context, q string) (assistant.Answer, error) {
	f.got = q
	return f.answer, f.err
}

var selicAnswer = assistant.Answer{
	Question: "Qual a Selic?", Answerable: true, Title: "Meta Selic", Explanation: "A meta Selic mais recente.",
	SQL:     "SELECT ref_date AS dia, value AS selic FROM mart.indicator_values WHERE indicator_id = 'selic'",
	Chart:   assistant.ChartSpec{Type: "line", X: "dia", Y: []string{"selic"}},
	Columns: []assistant.Column{{Name: "dia", Type: "date"}, {Name: "selic", Type: "number"}},
	Rows:    [][]any{{"2026-11-11", 13.75}},
}

// post faz um POST no router completo, com o assistente dado (nil = desligado).
func post(t *testing.T, asker Asker, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var opts []Option
	if asker != nil {
		opts = append(opts, WithAssistant(asker))
	}
	router := NewRouter(&fakeStore{}, slog.New(slog.NewTextHandler(io.Discard, nil)), []byte(testSpec), opts...)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestAsk_Answers(t *testing.T) {
	a := &fakeAsker{answer: selicAnswer}
	rec := post(t, a, "/v1/ask", `{"question":"Qual a Selic?"}`)

	if rec.Code != http.StatusOK || a.got != "Qual a Selic?" {
		t.Fatalf("status %d, pergunta recebida %q: %s", rec.Code, a.got, rec.Body)
	}
	for _, want := range []string{`"title":"Meta Selic"`, `"chart":{"type":"line","x":"dia","y":["selic"]}`, `"rows":[["2026-11-11",13.75]]`, `"cached":false`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("corpo sem %s: %s", want, rec.Body)
		}
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
}

func TestAsk_Errors(t *testing.T) {
	tests := []struct {
		name   string
		asker  Asker
		body   string
		status int
		code   string
	}{
		{"assistente desligado", nil, `{"question":"oi oi"}`, 503, "assistant_disabled"},
		{"corpo que não é JSON", &fakeAsker{}, `pergunta`, 400, "invalid_body"},
		{"campo desconhecido", &fakeAsker{}, `{"pergunta":"oi"}`, 400, "invalid_body"},
		{"corpo grande demais", &fakeAsker{}, `{"question":"` + strings.Repeat("a", maxAskBody) + `"}`, 400, "invalid_body"},
		{"pergunta curta", &fakeAsker{err: fmt.Errorf("%w: curta", assistant.ErrBadQuestion)}, `{"question":"a"}`, 400, "invalid_question"},
		{"limite do dia", &fakeAsker{err: assistant.ErrDailyLimit}, `{"question":"oi oi"}`, 429, "assistant_limit"},
		{"cota do Gemini", &fakeAsker{err: assistant.ErrQuotaExceeded}, `{"question":"oi oi"}`, 429, "assistant_limit"},
		{"plano ruim", &fakeAsker{err: fmt.Errorf("%w: x", assistant.ErrBadPlan)}, `{"question":"oi oi"}`, 422, "unanswerable"},
		{"provedor fora do ar", &fakeAsker{err: errors.New("gemini: status 500: chave X")}, `{"question":"oi oi"}`, 502, "assistant_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(t, tt.asker, "/v1/ask", tt.body)
			if rec.Code != tt.status || errorCode(t, rec) != tt.code {
				t.Errorf("status %d, corpo %s; quer %d e %s", rec.Code, rec.Body, tt.status, tt.code)
			}
			if strings.Contains(rec.Body.String(), "chave X") {
				t.Error("o detalhe do erro interno vazou na resposta")
			}
		})
	}
}

func TestAsk_OnlyPost(t *testing.T) {
	rec := do(t, &fakeStore{}, http.MethodGet, "/v1/ask")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /v1/ask: status %d, quer 405", rec.Code)
	}
}

func TestAsk_RateLimitPerIP(t *testing.T) {
	router := NewRouter(&fakeStore{}, slog.New(slog.NewTextHandler(io.Discard, nil)), []byte(testSpec),
		WithAssistant(&fakeAsker{answer: selicAnswer}),
		WithAskRateLimit(ratelimit.New(2, time.Minute), true))
	ask := func(xff string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/ask", strings.NewReader(`{"question":"Qual a Selic?"}`))
		req.Header.Set("X-Forwarded-For", xff)
		router.ServeHTTP(rec, req)
		return rec
	}
	// O primeiro item é do cliente (pode ser inventado); vale o último, do proxy.
	for i, xff := range []string{"9.9.9.1, 1.2.3.4", "9.9.9.2, 1.2.3.4"} {
		if rec := ask(xff); rec.Code != http.StatusOK {
			t.Fatalf("pergunta %d: status %d", i+1, rec.Code)
		}
	}
	rec := ask("9.9.9.3, 1.2.3.4")
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"rate_limited"`) || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("terceira: status %d, Retry-After %q, corpo %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body)
	}
	if rec := ask("1.2.3.5"); rec.Code != http.StatusOK {
		t.Fatalf("outro IP: status %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := post(t, nil, "/v1/ask", `{}`)
	for k, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, quer %q", k, got, want)
		}
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("a API não deve liberar CORS")
	}
}
