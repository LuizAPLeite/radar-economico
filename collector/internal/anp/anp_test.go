package anp

import (
	"slices"
	"testing"
	"time"
)

// Trecho com a mesma estrutura da página real: links de outras pastas,
// arquivos .zip e um link repetido.
const pageFixture = `
<ul>
  <li><a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/dsas/ca/ca-2026-01.zip">1º sem 2026</a></li>
  <li><a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/dsan/2026/10-gasolina-etanol.csv">Outubro</a></li>
  <li><a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/dsan/2026/10-diesel-gnv.csv">Outubro</a></li>
  <li><a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/qus/ultimas-4-semanas-glp.csv">4 semanas</a></li>
  <li><a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/dsan/2026/10-gasolina-etanol.csv">Outubro (de novo)</a></li>
</ul>`

func TestExtractLinks_OnlyMonthlyFilesWithoutRepetition(t *testing.T) {
	got := ExtractLinks([]byte(pageFixture))

	want := []string{
		"https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/dsan/2026/10-gasolina-etanol.csv",
		"https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/dsan/2026/10-diesel-gnv.csv",
	}
	if !slices.Equal(got, want) {
		t.Errorf("links:\n got: %v\nwant: %v", got, want)
	}
}

func TestFileKey(t *testing.T) {
	f := File{Group: GroupDieselGNV, Month: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)}
	if got := f.Key(); got != (Key{GroupDieselGNV, "2026-04"}) {
		t.Errorf("Key = %+v", got)
	}
}
