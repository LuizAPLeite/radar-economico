package assistant

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckSQL_Accepts(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"select simples", "SELECT 1", "SELECT 1"},
		{"minúsculas", "select id from mart.indicators", "select id from mart.indicators"},
		{"ponto e vírgula no fim", "SELECT 1;", "SELECT 1"},
		{"espaços e quebras de linha", "\n  SELECT 1 ;  \n", "SELECT 1"},
		{"with (CTE)", "WITH x AS (SELECT 1) SELECT * FROM x", "WITH x AS (SELECT 1) SELECT * FROM x"},
		{"select colado no parêntese", "SELECT(1)", "SELECT(1)"},
		{"subconsulta entre parênteses", "with t as (select 2) select * from t", "with t as (select 2) select * from t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckSQL(tt.in)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != tt.want {
				t.Errorf("CheckSQL(%q) = %q, quer %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCheckSQL_Rejects(t *testing.T) {
	tests := []struct {
		name, in string
	}{
		{"vazio", ""},
		{"só espaços e ponto e vírgula", "  ;  "},
		{"dois comandos", "SELECT 1; DROP TABLE mart.indicators"},
		{"dois pontos e vírgulas no fim", "SELECT 1;;"},
		{"comentário de linha", "SELECT 1 -- e mais nada"},
		{"comentário de bloco", "SELECT /* oi */ 1"},
		{"delete", "DELETE FROM mart.indicators"},
		{"update", "update mart.indicators set name = 'x'"},
		{"drop", "DROP TABLE mart.indicators"},
		{"insert", "INSERT INTO mart.indicators VALUES (1)"},
		{"set de papel", "SET ROLE radar"},
		{"copy", "COPY mart.indicators TO STDOUT"},
		{"palavra parecida", "SELECTED 1"},
		{"select no meio", "EXPLAIN SELECT 1"},
		{"longo demais", "SELECT " + strings.Repeat("1, ", MaxSQLLength)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckSQL(tt.in)
			if err == nil {
				t.Fatalf("CheckSQL(%q) aceitou (devolveu %q); quer erro", tt.in, got)
			}
			if !errors.Is(err, ErrUnsafeSQL) {
				t.Errorf("erro %v não embrulha ErrUnsafeSQL", err)
			}
		})
	}
}
