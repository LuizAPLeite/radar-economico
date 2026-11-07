package main

import (
	"context"
	"testing"
	"time"
)

func noop(context.Context, int64) error { return nil }

func TestSelectSources(t *testing.T) {
	all := []source{{"bcb_sgs", noop}, {"ibge", noop}}

	tests := []struct {
		name    string
		names   []string
		want    []string
		wantErr bool
	}{
		{"vazio seleciona todas", nil, []string{"bcb_sgs", "ibge"}, false},
		{"uma só", []string{"ibge"}, []string{"ibge"}, false},
		{"respeita a ordem pedida", []string{"ibge", "bcb_sgs"}, []string{"ibge", "bcb_sgs"}, false},
		{"fonte desconhecida é erro", []string{"bcb"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectSources(all, tt.names)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, quer %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].name != tt.want[i] {
					t.Errorf("[%d] = %q, quer %q", i, got[i].name, tt.want[i])
				}
			}
		})
	}
}

func TestDefaultANPStart(t *testing.T) {
	tests := map[string]struct{ today, want time.Time }{
		"meio do ano": {date(2026, time.November, 7), date(2025, time.November, 1)},
		"janeiro":     {date(2026, time.January, 31), date(2025, time.January, 1)},
	}
	for name, tt := range tests {
		if got := defaultANPStart(tt.today); !got.Equal(tt.want) {
			t.Errorf("%s: defaultANPStart(%s) = %s, quer %s", name,
				tt.today.Format(time.DateOnly), got.Format(time.DateOnly), tt.want.Format(time.DateOnly))
		}
	}
}

func TestLoadConfig_ANPStartMonth(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")

	t.Setenv("ANP_START_MONTH", "2025-06")
	cfg, err := loadConfig()
	if err != nil || !cfg.anpStart.Equal(date(2025, time.June, 1)) {
		t.Errorf("anpStart = %v, err = %v", cfg.anpStart, err)
	}

	t.Setenv("ANP_START_MONTH", "06/2025")
	if _, err := loadConfig(); err == nil {
		t.Error("esperava erro para mês fora do formato AAAA-MM")
	}
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestLoadConfig_Sources(t *testing.T) {
	// t.Setenv define a variável só durante este teste (e restaura no fim).
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("COLLECTOR_SOURCES", " ibge, bcb_sgs ,ibge,")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.sources) != 2 || cfg.sources[0] != "ibge" || cfg.sources[1] != "bcb_sgs" {
		t.Errorf("sources = %q, quer [ibge bcb_sgs] (sem espaços nem repetidos)", cfg.sources)
	}
}
