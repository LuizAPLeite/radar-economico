package main

import (
	"testing"
	"time"
)

func TestDurationEnv(t *testing.T) {
	const fallback = 30 * time.Minute
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{value: "", want: fallback}, // sem a variável, vale o padrão
		{value: "1m", want: time.Minute},
		{value: "1h30m", want: 90 * time.Minute},
		{value: "0", want: 0},         // desliga o cache
		{value: "30", wantErr: true},  // sem unidade: 30 o quê?
		{value: "-5m", wantErr: true}, // negativo não faz sentido
		{value: "meia hora", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			// t.Setenv define a variável só durante este teste.
			t.Setenv("CACHE_TTL", tt.value)

			got, err := durationEnv("CACHE_TTL", fallback)
			if (err != nil) != tt.wantErr {
				t.Fatalf("erro = %v, quer erro: %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("durationEnv(%q) = %v, quer %v", tt.value, got, tt.want)
			}
		})
	}
}
