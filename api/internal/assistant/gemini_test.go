package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGemini sobe um servidor HTTP local que faz o papel da API do Gemini:
// guarda a requisição recebida e responde com status e corpo dados.
func fakeGemini(t *testing.T, status int, body string) (*GeminiPlanner, *http.Request, *map[string]any) {
	t.Helper()
	var gotReq http.Request
	gotBody := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = *r
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	g := NewGeminiPlanner("chave-de-teste", "gemini-teste", func() string { return "2026-11-12" })
	g.BaseURL = srv.URL
	return g, &gotReq, &gotBody
}

// completed monta uma resposta da API com o texto dado como saída do modelo,
// depois de uma etapa de raciocínio (que precisa ser ignorada).
func completed(text string) string {
	quoted, _ := json.Marshal(text)
	return `{"id":"x","status":"completed","steps":[` +
		`{"type":"thought","content":[{"type":"text","text":"pensando"}]},` +
		`{"type":"model_output","content":[{"type":"text","text":` + string(quoted) + `}]}],` +
		`"usage":{"total_tokens":10}}`
}

const planJSON = `{"answerable":true,"sql":"SELECT 1","title":"Um","explanation":"Teste.",` +
	`"chart":{"type":"none","x":"","y":[]}}`

func TestGeminiPlan_SendsTheRequestAndReadsThePlan(t *testing.T) {
	g, req, body := fakeGemini(t, http.StatusOK, completed(planJSON))

	plan, err := g.Plan(context.Background(), "Qual a Selic hoje?")
	if err != nil {
		t.Fatal(err)
	}
	want := Plan{Answerable: true, SQL: "SELECT 1", Title: "Um", Explanation: "Teste.", Chart: ChartSpec{Type: "none", Y: []string{}}}
	if plan.SQL != want.SQL || plan.Title != want.Title || !plan.Answerable || plan.Chart.Type != "none" {
		t.Errorf("plano = %+v", plan)
	}

	if req.Method != http.MethodPost || req.Header.Get("x-goog-api-key") != "chave-de-teste" ||
		req.Header.Get("Api-Revision") != geminiAPIRevision {
		t.Errorf("requisição: %s, cabeçalhos %v", req.Method, req.Header)
	}
	if strings.Contains(req.URL.String(), "chave-de-teste") {
		t.Error("a chave não pode ir na URL")
	}
	b := *body
	if b["model"] != "gemini-teste" || b["store"] != false || b["system_instruction"] != systemPrompt {
		t.Errorf("corpo: model=%v store=%v", b["model"], b["store"])
	}
	if input, _ := b["input"].(string); !strings.Contains(input, "2026-11-12") || !strings.Contains(input, "Qual a Selic hoje?") {
		t.Errorf("input = %q", b["input"])
	}
	format, _ := b["response_format"].(map[string]any)
	if format["mime_type"] != "application/json" || format["schema"] == nil {
		t.Errorf("response_format = %v", format)
	}
}

func TestGeminiPlan_QuotaIsAnErrorOfItsOwn(t *testing.T) {
	g, _, _ := fakeGemini(t, http.StatusTooManyRequests, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}`)
	if _, err := g.Plan(context.Background(), "x"); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("erro = %v, quer ErrQuotaExceeded", err)
	}
}

func TestGeminiPlan_Failures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string // trecho da mensagem de erro
	}{
		{"erro do servidor", http.StatusInternalServerError, `{"error":"falhou"}`, "status 500"},
		{"corpo não é JSON", http.StatusOK, `<html>`, "não é JSON"},
		{"interação incompleta", http.StatusOK, `{"status":"incomplete","steps":[]}`, `"incomplete"`},
		{"sem texto", http.StatusOK, `{"status":"completed","steps":[]}`, "sem texto"},
		{"plano fora do formato", http.StatusOK, completed(`{"sql": 42}`), "JSON esperado"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _, _ := fakeGemini(t, tt.status, tt.body)
			_, err := g.Plan(context.Background(), "x")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("erro = %v, quer algo com %q", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), "chave-de-teste") {
				t.Error("a chave apareceu na mensagem de erro")
			}
		})
	}
}
