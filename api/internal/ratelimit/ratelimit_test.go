package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	clock := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	l := New(2, time.Minute)
	l.now = func() time.Time { return clock }

	for i := range 2 {
		if ok, _ := l.Allow("1.2.3.4"); !ok {
			t.Fatalf("requisição %d recusada; quer aceita", i+1)
		}
	}
	ok, wait := l.Allow("1.2.3.4")
	if ok || wait != time.Minute {
		t.Fatalf("terceira: ok=%v, espera %v; quer recusada, 1m", ok, wait)
	}
	// Outro cliente tem a própria conta.
	if ok, _ := l.Allow("5.6.7.8"); !ok {
		t.Fatal("outro IP recusado")
	}
	// A janela vira e o primeiro volta a poder.
	clock = clock.Add(time.Minute)
	if ok, _ := l.Allow("1.2.3.4"); !ok {
		t.Fatal("recusada depois de virar a janela")
	}
}

func TestLimiter_DropsExpiredKeys(t *testing.T) {
	clock := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	l := New(1, time.Minute)
	l.now = func() time.Time { return clock }
	for i := range maxKeys {
		l.Allow(string(rune(i)))
	}
	clock = clock.Add(time.Minute)
	l.Allow("novo")
	if len(l.hits) != 1 {
		t.Fatalf("%d chaves guardadas; quer só a nova", len(l.hits))
	}
}
