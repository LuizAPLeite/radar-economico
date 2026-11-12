package assistant

import (
	"context"
	"errors"
)

// Plan é o que o modelo de linguagem devolve para uma pergunta: a consulta
// que a responde e como mostrar o resultado.
type Plan struct {
	// Answerable é false quando os dados da mart não respondem à pergunta
	// (nesse caso SQL vem vazio e Explanation diz o motivo).
	Answerable  bool      `json:"answerable"`
	SQL         string    `json:"sql"`
	Title       string    `json:"title"`
	Explanation string    `json:"explanation"`
	Chart       ChartSpec `json:"chart"`
}

// ChartSpec diz como desenhar o resultado: as colunas do eixo X e do Y.
type ChartSpec struct {
	Type string   `json:"type"` // "line", "bar" ou "none"
	X    string   `json:"x"`
	Y    []string `json:"y"`
}

// Planner transforma uma pergunta num Plan. É a interface do provedor de
// LLM: o Gemini hoje (GeminiPlanner); outro amanhã, sem mudar quem a usa.
// Nos testes, um Planner falso devolve planos prontos sem rede.
type Planner interface {
	Plan(ctx context.Context, question string) (Plan, error)
}

// ErrQuotaExceeded é devolvido quando o provedor recusa por cota (HTTP 429):
// no plano grátis, o limite de perguntas por minuto ou por dia acabou.
var ErrQuotaExceeded = errors.New("cota do modelo de linguagem esgotada")
