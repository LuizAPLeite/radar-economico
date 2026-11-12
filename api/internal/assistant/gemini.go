package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiBaseURL é o endpoint da API Interactions do Gemini (formato conferido
// em 2026-11-12 em aistudio.google.com/docs/text-generation e structured-output).
const GeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/interactions"

// geminiAPIRevision fixa a versão do formato da API: uma revisão nova não
// muda as respostas que este código sabe ler sem que ninguém perceba.
const geminiAPIRevision = "2026-05-20"

// GeminiPlanner é o Planner que pergunta ao Gemini. HTTP puro, sem SDK: são
// uma requisição e uma resposta JSON, e assim não há dependência nova.
type GeminiPlanner struct {
	HTTPClient *http.Client
	APIKey     string
	Model      string // ex.: gemini-3.8-flash
	BaseURL    string // GeminiBaseURL; os testes trocam por um servidor falso
	// Today devolve a data de hoje (AAAA-MM-DD) em Brasília, mandada junto com
	// a pergunta para "este ano", "último mês" etc. Os testes fixam a data.
	Today func() string
}

// Conferência em tempo de compilação: GeminiPlanner implementa Planner.
var _ Planner = (*GeminiPlanner)(nil)

func NewGeminiPlanner(apiKey, model string, today func() string) *GeminiPlanner {
	return &GeminiPlanner{
		// O modelo pensa antes de responder: alguns segundos são normais.
		HTTPClient: &http.Client{Timeout: 45 * time.Second},
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    GeminiBaseURL,
		Today:      today,
	}
}

type geminiRequest struct {
	Model             string               `json:"model"`
	SystemInstruction string               `json:"system_instruction"`
	Input             string               `json:"input"`
	Store             bool                 `json:"store"` // false: o Google não guarda a conversa
	ResponseFormat    geminiResponseFormat `json:"response_format"`
	GenerationConfig  geminiGenConfig      `json:"generation_config"`
}

type geminiResponseFormat struct {
	Type     string         `json:"type"`
	MimeType string         `json:"mime_type"`
	Schema   map[string]any `json:"schema"`
}

type geminiGenConfig struct {
	ThinkingLevel   string `json:"thinking_level"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

type geminiResponse struct {
	Status string `json:"status"`
	Steps  []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"steps"`
}

// Plan manda a pergunta ao Gemini e lê o plano da resposta estruturada.
func (g *GeminiPlanner) Plan(ctx context.Context, question string) (Plan, error) {
	body, err := json.Marshal(geminiRequest{
		Model:             g.Model,
		SystemInstruction: systemPrompt,
		Input:             fmt.Sprintf("Data de hoje: %s\nPergunta: %s", g.Today(), question),
		Store:             false,
		ResponseFormat:    geminiResponseFormat{Type: "text", MimeType: "application/json", Schema: planSchema},
		// "low": SQL sobre poucas tabelas não precisa de raciocínio longo, e
		// pensar menos responde mais rápido.
		GenerationConfig: geminiGenConfig{ThinkingLevel: "low", MaxOutputTokens: 2000},
	})
	if err != nil {
		return Plan{}, fmt.Errorf("gemini: montando a requisição: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL, bytes.NewReader(body))
	if err != nil {
		return Plan{}, fmt.Errorf("gemini: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.APIKey) // no cabeçalho, nunca na URL (iria parar em logs)
	req.Header.Set("Api-Revision", geminiAPIRevision)

	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return Plan{}, fmt.Errorf("gemini: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Plan{}, fmt.Errorf("gemini: lendo a resposta: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return Plan{}, ErrQuotaExceeded
	}
	if resp.StatusCode != http.StatusOK {
		// O corpo do erro vai para o log do servidor (sem a chave, que só
		// existe no cabeçalho da requisição), nunca para o visitante.
		return Plan{}, fmt.Errorf("gemini: status %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}

	var parsed geminiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Plan{}, fmt.Errorf("gemini: resposta não é JSON: %w", err)
	}
	if parsed.Status != "completed" {
		return Plan{}, fmt.Errorf("gemini: interação terminou como %q", parsed.Status)
	}
	text := lastText(parsed)
	if text == "" {
		return Plan{}, errors.New("gemini: resposta sem texto")
	}

	var plan Plan
	if err := json.Unmarshal([]byte(text), &plan); err != nil {
		return Plan{}, fmt.Errorf("gemini: o plano não é o JSON esperado: %w", err)
	}
	return plan, nil
}

// lastText devolve o texto da última saída do modelo (as etapas de
// raciocínio, quando vêm, têm outro tipo e ficam de fora).
func lastText(r geminiResponse) string {
	for i := len(r.Steps) - 1; i >= 0; i-- {
		if r.Steps[i].Type != "model_output" {
			continue
		}
		var b strings.Builder
		for _, c := range r.Steps[i].Content {
			if c.Type == "text" {
				b.WriteString(c.Text)
			}
		}
		return b.String()
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
