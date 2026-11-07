package main

import (
	"context"
	"testing"
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
