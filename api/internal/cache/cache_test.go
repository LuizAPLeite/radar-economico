package cache

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeClock é um relógio que só anda quando o teste manda: assim dá para
// testar a expiração sem esperar 10 minutos de verdade.
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newTestCache(ttl time.Duration) (*Cache[string], *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	c := New[string](ttl)
	c.now = clock.now
	return c, clock
}

func TestGet_EmptyCache(t *testing.T) {
	c, _ := newTestCache(time.Minute)

	v, ok := c.Get("nada")
	if ok || v != "" {
		t.Fatalf("Get num cache vazio = (%q, %v), quer (\"\", false)", v, ok)
	}
}

func TestSetThenGet(t *testing.T) {
	c, _ := newTestCache(time.Minute)

	c.Set("selic", "13,75")

	v, ok := c.Get("selic")
	if !ok || v != "13,75" {
		t.Fatalf("Get = (%q, %v), quer (\"13,75\", true)", v, ok)
	}
}

func TestKeysAreIndependent(t *testing.T) {
	c, _ := newTestCache(time.Minute)

	c.Set("selic", "13,75")
	c.Set("ipca", "0,82")

	if v, _ := c.Get("selic"); v != "13,75" {
		t.Errorf("selic = %q", v)
	}
	if v, _ := c.Get("ipca"); v != "0,82" {
		t.Errorf("ipca = %q", v)
	}
}

func TestExpiresAfterTTL(t *testing.T) {
	c, clock := newTestCache(10 * time.Minute)
	c.Set("selic", "13,75")

	clock.advance(10*time.Minute - time.Nanosecond)
	if _, ok := c.Get("selic"); !ok {
		t.Fatal("1ns antes do fim do TTL o valor ainda deveria valer")
	}

	clock.advance(time.Nanosecond)
	if v, ok := c.Get("selic"); ok || v != "" {
		t.Fatalf("ao completar o TTL deveria ter expirado; Get = (%q, %v)", v, ok)
	}
}

func TestSetRenewsExpiration(t *testing.T) {
	c, clock := newTestCache(10 * time.Minute)
	c.Set("selic", "13,50")

	clock.advance(8 * time.Minute)
	c.Set("selic", "13,75") // valor novo, prazo novo

	clock.advance(8 * time.Minute) // 16 min do primeiro Set, 8 do segundo
	if v, ok := c.Get("selic"); !ok || v != "13,75" {
		t.Fatalf("Get = (%q, %v), quer (\"13,75\", true)", v, ok)
	}
}

// Várias goroutines usando o mesmo cache. Sem o mutex, o Go costuma abortar
// com "concurrent map writes"; com `go test -race`, o detector acusa sempre.
func TestConcurrentUse(t *testing.T) {
	c := New[int](time.Minute)
	var wg sync.WaitGroup
	for i := range 100 {
		// wg.Go roda a função numa goroutine nova e o wg.Wait() espera todas.
		wg.Go(func() {
			key := strconv.Itoa(i % 5)
			c.Set(key, i)
			c.Get(key)
		})
	}
	wg.Wait()
}
