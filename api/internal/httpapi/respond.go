package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSON serializa v e escreve a resposta. Serializar ANTES de escrever o
// status permite responder 500 se a serialização falhar (depois que o status
// foi enviado, não dá mais para trocá-lo).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	body, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"erro ao gerar a resposta"}}`))
		return
	}
	w.WriteHeader(status)
	// Um erro aqui só acontece se o cliente desconectou; não há o que fazer.
	_, _ = w.Write(body)
}

// errorJSON é o formato de todas as respostas de erro:
//
//	{"error": {"code": "not_found", "message": "indicador \"x\" não encontrado"}}
//
// code é estável, para programas; message é para pessoas.
type errorJSON struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorJSON{Error: errorDetail{Code: code, Message: message}})
}
