package anp

import (
	"testing"
	"time"
)

const base = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc"

func month(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }

// Os endereços abaixo são nomes REAIS publicados pela ANP (conferidos em
// 2026-11-07), com toda a bagunça de padrões.
func TestParseFileURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOK    bool
		wantGroup string
		wantMonth time.Time
	}{
		// Padrão de 2023 a 2025: o mês no FIM do nome.
		{"2023, mês no fim", base + "/dsan/2023/precos-gasolina-etanol-01.csv", true, GroupGasolinaEtanol, month(2023, time.January)},
		{"2024, diesel, dezembro", base + "/dsan/2024/precos-diesel-gnv-12.csv", true, GroupDieselGNV, month(2024, time.December)},

		// Padrão de 2026: o mês no COMEÇO, com nomes variados.
		{"2026, nome longo", base + "/dsan/2026/01-dados-abertos-precos-gasolina-etanol.csv", true, GroupGasolinaEtanol, month(2026, time.January)},
		{"2026, nome com erro de digitação", base + "/dsan/2026/02-cados-abertos-preco-gasolina-etanol.csv", true, GroupGasolinaEtanol, month(2026, time.February)},
		{"2026, nome com a data no meio", base + "/dsan/2026/06-dados-abertos-precos-2026-06-diesel-gnv.csv", true, GroupDieselGNV, month(2026, time.June)},
		{"2026, nome curto", base + "/dsan/2026/10-gasolina-etanol.csv", true, GroupGasolinaEtanol, month(2026, time.October)},

		// O que NÃO queremos.
		{"GLP no padrão antigo", base + "/dsan/2023/precos-glp-05.csv", false, "", time.Time{}},
		{"GLP no padrão novo", base + "/dsan/2026/10-glp.csv", false, "", time.Time{}},
		{"arquivo semestral (outra pasta)", base + "/dsas/ca/ca-2026-01.zip", false, "", time.Time{}},
		{"mês inválido", base + "/dsan/2026/13-gasolina-etanol.csv", false, "", time.Time{}},
		{"sem mês", base + "/dsan/2026/precos-gasolina-etanol.csv", false, "", time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseFileURL(tt.url)

			if ok != tt.wantOK {
				t.Fatalf("ok = %v, quer %v (File = %+v)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if got.Group != tt.wantGroup {
				t.Errorf("Group = %q, quer %q", got.Group, tt.wantGroup)
			}
			if !got.Month.Equal(tt.wantMonth) {
				t.Errorf("Month = %s, quer %s", got.Month.Format(time.DateOnly), tt.wantMonth.Format(time.DateOnly))
			}
			if got.URL != tt.url {
				t.Errorf("URL = %q, quer o próprio endereço", got.URL)
			}
		})
	}
}

func TestDiscover_SortsAndSeparatesUnknown(t *testing.T) {
	html := []byte(`
		<a href="` + base + `/dsan/2026/10-gasolina-etanol.csv">out</a>
		<a href="` + base + `/dsan/2026/10-glp.csv">glp</a>
		<a href="` + base + `/dsan/2025/precos-diesel-gnv-12.csv">dez</a>
		<a href="` + base + `/dsan/2026/10-diesel-gnv.csv">out</a>`)

	files, unknown := Discover(html)

	want := []Key{
		{GroupDieselGNV, "2025-12"},
		{GroupDieselGNV, "2026-10"},
		{GroupGasolinaEtanol, "2026-10"},
	}
	if len(files) != len(want) {
		t.Fatalf("files = %+v", files)
	}
	for i, k := range want {
		if files[i].Key() != k {
			t.Errorf("files[%d] = %+v, quer %+v", i, files[i].Key(), k)
		}
	}
	if len(unknown) != 1 {
		t.Errorf("unknown = %v, quer só o de GLP", unknown)
	}
}
