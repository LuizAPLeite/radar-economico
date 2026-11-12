package collect

import (
	"strings"
	"testing"
)

func TestValidateFREDCSV(t *testing.T) {
	validate := validateFREDCSV("DCOILBRENTEU")

	tests := []struct {
		name    string
		body    string
		wantErr string // "" = sem erro; senão, um trecho da mensagem
	}{
		{"CSV válido", "observation_date,DCOILBRENTEU\n1987-05-20,18.63\n1987-05-21,18.45\n", ""},
		{"quebras de linha do Windows", "observation_date,DCOILBRENTEU\r\n1987-05-20,18.63\r\n", ""},
		{"dia sem cotação também é dado", "observation_date,DCOILBRENTEU\n2026-11-03,\n", ""},
		{"outra série", "observation_date,DCOILWTICO\n1987-05-20,18.63\n", "DCOILWTICO"},
		{"página de erro em HTML", "<!DOCTYPE html><html>erro</html>", "<!DOCTYPE html>"},
		{"só o cabeçalho", "observation_date,DCOILBRENTEU\n", "dados"},
		{"cabeçalho e linhas em branco", "observation_date,DCOILBRENTEU\n\n  \n", "dados"},
		{"vazio", "", "cabeçalho"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate([]byte(tt.body))
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("erro inesperado: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Errorf("quer erro mencionando %q, veio nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("erro %q não menciona %q", err, tt.wantErr)
			}
		})
	}
}

func TestFREDFiles(t *testing.T) {
	if len(FREDFiles) != 1 || FREDFiles[0].Name != "DCOILBRENTEU" ||
		FREDFiles[0].URL != "https://fred.stlouisfed.org/graph/fredgraph.csv?id=DCOILBRENTEU" {
		t.Errorf("FREDFiles = %+v", FREDFiles)
	}
}
