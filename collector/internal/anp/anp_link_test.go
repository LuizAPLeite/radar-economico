package anp

import "testing"

// AM-08: só links HTTPS do www.gov.br entram na coleta.
func TestLinkPattern_OnlyHTTPSGovBR(t *testing.T) {
	tests := []struct {
		html string
		ok   bool
	}{
		{`href="https://www.gov.br/anp/pt-br/arquivos/shpc/dsan/2026/precos-gasolina-etanol-10.csv"`, true},
		{`href="http://www.gov.br/anp/pt-br/arquivos/shpc/dsan/2026/precos-gasolina-etanol-10.csv"`, false},
		{`href="https://www.gov.br.example.com/x/shpc/dsan/2026/precos-gasolina-etanol-10.csv"`, false},
		{`href="https://evil.example/www.gov.br/shpc/dsan/2026/precos-gasolina-etanol-10.csv"`, false},
	}
	for _, tt := range tests {
		if got := linkPattern.MatchString(tt.html); got != tt.ok {
			t.Errorf("%s: casou=%v, quer %v", tt.html, got, tt.ok)
		}
	}
}
