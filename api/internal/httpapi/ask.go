package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/LuizAPLeite/radar-economico/api/internal/assistant"
)

// Asker responde perguntas em português sobre a mart (assistant.Service).
type Asker interface {
	Ask(ctx context.Context, question string) (assistant.Answer, error)
}

// Option ajusta o router na criação. É o padrão "functional options" do Go:
// recursos opcionais entram como funções, e quem não precisa deles (os
// testes das outras rotas) continua chamando NewRouter do mesmo jeito.
type Option func(*handlers)

// WithAssistant liga a rota POST /v1/ask. Sem ela (sem chave do LLM), a
// rota responde 503.
func WithAssistant(a Asker) Option {
	return func(h *handlers) { h.asker = a }
}

// maxAskBody limita o corpo da requisição: a pergunta tem até 300 caracteres.
const maxAskBody = 4 << 10 // 4 KB

type askRequest struct {
	Question string `json:"question"`
}

// ask responde POST /v1/ask com {"question": "..."}: o LLM escreve o SQL, a
// API confere, executa na mart com permissões mínimas e devolve os dados.
func (h *handlers) ask(w http.ResponseWriter, r *http.Request) {
	// Cada pergunta é diferente e pode gastar a cota do LLM: nada de cache
	// em proxies ou no navegador.
	w.Header().Set("Cache-Control", "no-store")
	if h.asker == nil {
		writeError(w, http.StatusServiceUnavailable, "assistant_disabled",
			"o assistente não está ligado neste servidor")
		return
	}

	var req askRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAskBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body",
			`corpo inválido: envie um JSON como {"question": "Qual a Selic hoje?"}`)
		return
	}

	answer, err := h.asker.Ask(r.Context(), req.Question)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, answer)
	case errors.Is(err, assistant.ErrBadQuestion):
		writeError(w, http.StatusBadRequest, "invalid_question", "a pergunta precisa ter de 3 a 300 caracteres")
	case errors.Is(err, assistant.ErrDailyLimit), errors.Is(err, assistant.ErrQuotaExceeded):
		writeError(w, http.StatusTooManyRequests, "assistant_limit",
			"o assistente atingiu o limite de perguntas por agora; tente mais tarde ou use uma pergunta de exemplo")
	case errors.Is(err, assistant.ErrBadPlan):
		writeError(w, http.StatusUnprocessableEntity, "unanswerable",
			"não consegui montar uma consulta para essa pergunta; tente reformular")
	default:
		// Falha do provedor (rede, 5xx): o detalhe fica no log, não na resposta.
		h.logger.Error("assistente indisponível", "request_id", middleware.GetReqID(r.Context()), "err", err)
		writeError(w, http.StatusBadGateway, "assistant_unavailable",
			"o assistente está indisponível no momento; tente de novo em instantes")
	}
}
