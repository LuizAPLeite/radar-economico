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
		// Nomes parecidos com os proibidos, mas legítimos, continuam passando.
		{"coluna com nome parecido", "SELECT setting_config, sleep_hours FROM t", "SELECT setting_config, sleep_hours FROM t"},
		{"funções comuns", "SELECT date_trunc('month', ref_date), round(avg(value)::numeric, 2) FROM mart.indicator_values GROUP BY 1",
			"SELECT date_trunc('month', ref_date), round(avg(value)::numeric, 2) FROM mart.indicator_values GROUP BY 1"},
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
		// AM-01: a fuga do papel do assistente, e variações para driblar a regra.
		{"fuga do papel (AM-01)", "SELECT set_config('role', 'neondb_owner', true), query_to_xml('SELECT 1', true, false, '')"},
		{"set_config com esquema", "SELECT pg_catalog.set_config('role', 'x', true)"},
		{"set_config entre aspas", `SELECT "set_config"('role', 'x', true)`},
		{"set_config com espaço", "SELECT SET_CONFIG ('role', 'x', true)"},
		{"query_to_xml sozinho", "SELECT query_to_xml('SELECT * FROM raw.collection_runs', true, false, '')"},
		{"table_to_xml", "SELECT table_to_xml('raw.collection_runs', true, false, '')"},
		{"dblink", "SELECT * FROM dblink('host=x', 'SELECT 1') AS t(a int)"},
		{"ler arquivo do servidor", "SELECT pg_read_file('/etc/passwd')"},
		{"dormir", "WITH x AS (SELECT pg_sleep(10)) SELECT * FROM x"},
		{"escape Unicode", `SELECT U&"\0073et_config"('role', 'x', true)`},
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
