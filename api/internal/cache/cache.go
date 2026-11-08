// Package cache guarda valores em memória por um tempo limitado (TTL).
//
// A API usa o cache para não ir ao banco a cada requisição: os dados da mart
// mudam no máximo uma vez por dia, e no plano grátis do Neon cada minuto de
// banco acordado conta na cota mensal.
package cache

import (
	"sync"
	"time"
)

// Cache guarda valores do tipo V, cada um válido por ttl.
//
// [V any] é um parâmetro de tipo (generics): o mesmo código guarda qualquer
// tipo de valor, e o compilador confere o tipo em cada uso. Por exemplo,
// cache.New[string](time.Minute) cria um cache que só aceita strings.
//
// O servidor HTTP atende cada requisição numa goroutine própria, então várias
// podem usar o mesmo Cache ao mesmo tempo: o mutex garante que só uma mexe no
// mapa por vez. (Um map do Go não pode ser lido e escrito em paralelo.)
type Cache[V any] struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time // relógio; os testes trocam por um relógio falso
	entries map[string]entry[V]
}

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// New cria um cache em que cada valor vale por ttl.
func New[V any](ttl time.Duration) *Cache[V] {
	return &Cache[V]{
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[string]entry[V]),
	}
}

// Get devolve o valor guardado em key e true, se ele existir e ainda não
// tiver expirado. Caso contrário, devolve o valor zero de V e false.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok || !c.now().Before(e.expiresAt) {
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set guarda value em key, válido por ttl a partir de agora.
func (c *Cache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry[V]{value: value, expiresAt: c.now().Add(c.ttl)}
}
