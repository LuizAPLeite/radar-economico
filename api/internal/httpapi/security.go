package httpapi

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/LuizAPLeite/radar-economico/api/internal/ratelimit"
)

// securityHeaders põe em toda resposta os cabeçalhos que dizem ao navegador
// para não fazer nada além de ler o JSON: não adivinhar o tipo do conteúdo,
// não abrir a resposta dentro de outra página, não rodar script nenhum e só
// falar com a API por HTTPS.
//
// Sem CORS de propósito: a API não manda Access-Control-Allow-Origin, então
// o navegador não deixa a página de outro site ler as respostas. Quem fala
// com a API é o servidor do Next, não o navegador.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		h.Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

// WithAskRateLimit limita as perguntas ao assistente por IP, além do limite
// diário que vale para todos: assim uma pessoa sozinha não esgota o dia.
// trustProxy diz que a API roda atrás de um proxy (o do Render), que acrescenta
// o IP de quem conectou no fim do X-Forwarded-For.
func WithAskRateLimit(l *ratelimit.Limiter, trustProxy bool) Option {
	return func(h *handlers) {
		h.askLimiter = l
		h.trustProxy = trustProxy
	}
}

// allowAsk aplica o limite por IP. Responde 429 e devolve false quando o
// cliente passou do limite.
func (h *handlers) allowAsk(w http.ResponseWriter, r *http.Request) bool {
	if h.askLimiter == nil {
		return true
	}
	ok, wait := h.askLimiter.Allow(clientIP(r, h.trustProxy))
	if ok {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
	writeError(w, http.StatusTooManyRequests, "rate_limited",
		"muitas perguntas em pouco tempo; espere um minuto e tente de novo")
	return false
}

// clientIP devolve o IP de quem fez a requisição. Atrás do proxy, vale o
// ÚLTIMO item do X-Forwarded-For, que o próprio proxy escreveu: os
// anteriores vêm do cliente e podem ser inventados para driblar o limite.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
