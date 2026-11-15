// Package ratelimit limita quantas requisições cada cliente faz por janela de
// tempo, em memória (uma instância da API, como o cache).
package ratelimit

import (
	"sync"
	"time"
)

// maxKeys limita a memória: passando disso, as janelas vencidas são
// descartadas antes de criar uma nova.
const maxKeys = 10_000

// Limiter conta requisições por chave (o IP do cliente) em janelas fixas:
// cada chave pode fazer até Limit requisições a cada Window. É o algoritmo
// mais simples que resolve o problema (impedir que uma pessoa sozinha
// esgote o limite diário do assistente); a borda entre duas janelas pode
// deixar passar até 2×Limit em sequência, o que aqui não importa.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string]*counter
}

type counter struct {
	start time.Time
	n     int
}

// New cria um limitador de limit requisições por window para cada chave.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, hits: map[string]*counter{}}
}

// Allow registra uma requisição da chave e diz se ela pode seguir. Quando
// não pode, devolve também quanto falta para a janela da chave virar.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	c, ok := l.hits[key]
	if !ok || now.Sub(c.start) >= l.window {
		if !ok && len(l.hits) >= maxKeys {
			l.dropExpired(now)
		}
		l.hits[key] = &counter{start: now, n: 1}
		return true, 0
	}
	if c.n >= l.limit {
		return false, c.start.Add(l.window).Sub(now)
	}
	c.n++
	return true, 0
}

// dropExpired apaga as janelas que já venceram. Chamada com o mutex travado.
func (l *Limiter) dropExpired(now time.Time) {
	for k, c := range l.hits {
		if now.Sub(c.start) >= l.window {
			delete(l.hits, k)
		}
	}
}
