// Package anp identifica, na página de dados abertos da ANP, os arquivos
// mensais da série histórica de preços de combustíveis por posto.
//
// Formato conferido em 2026-11-07:
//   - um CSV por mês e por grupo de produtos, na pasta shpc/dsan/AAAA/;
//   - os nomes NÃO seguem um padrão: "precos-gasolina-etanol-01.csv"
//     (2023 a 2025) e, em 2026, "01-dados-abertos-precos-gasolina-etanol.csv",
//     "02-cados-abertos-preco-gasolina-etanol.csv" (sic),
//     "06-dados-abertos-precos-2026-06-gasolina-etanol.csv" e
//     "10-gasolina-etanol.csv";
//   - meses podem faltar (abril/2026 não foi publicado);
//   - o servidor responde 403 a HEAD: só GET funciona.
package anp

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// DatasetPageURL é a página que lista os arquivos.
const DatasetPageURL = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/serie-historica-de-precos-de-combustiveis"

// Grupos de produtos publicados em arquivos separados. O GLP (botijão de
// 13 kg, outra cobertura) fica de fora.
const (
	GroupGasolinaEtanol = "gasolina-etanol"
	GroupDieselGNV      = "diesel-gnv"
)

// Groups lista os grupos coletados.
var Groups = []string{GroupGasolinaEtanol, GroupDieselGNV}

// File é um arquivo mensal de preços.
type File struct {
	Group string
	Month time.Time // primeiro dia do mês de referência, em UTC
	URL   string
}

// Key identifica um arquivo pelo grupo e pelo mês. O mês vai como texto
// ("2026-10"), e não como time.Time: um map compara as chaves com ==, e dois
// time.Time do mesmo instante podem ser diferentes com == (o fuso também
// entra na comparação). É uma armadilha clássica do Go.
type Key struct {
	Group string
	Month string // AAAA-MM
}

// Key devolve a chave do arquivo.
func (f File) Key() Key {
	return Key{Group: f.Group, Month: f.Month.Format("2006-01")}
}

// linkPattern acha os links de arquivos mensais (pasta dsan/AAAA/) no HTML.
var linkPattern = regexp.MustCompile(`href="(https?://[^"]+/shpc/dsan/\d{4}/[^"/]+\.csv)"`)

// ExtractLinks devolve os links de arquivos mensais encontrados no HTML,
// sem repetições e na ordem em que aparecem.
func ExtractLinks(html []byte) []string {
	var links []string
	seen := make(map[string]bool)
	for _, m := range linkPattern.FindAllSubmatch(html, -1) {
		link := string(m[1])
		if !seen[link] {
			seen[link] = true
			links = append(links, link)
		}
	}
	return links
}

// Discover separa os links reconhecidos (grupo e mês identificados por
// ParseFileURL) dos demais, como os de GLP. Os arquivos saem em ordem de mês
// e grupo.
func Discover(html []byte) (files []File, unknown []string) {
	for _, link := range ExtractLinks(html) {
		if f, ok := ParseFileURL(link); ok {
			files = append(files, f)
		} else {
			unknown = append(unknown, link)
		}
	}
	slices.SortFunc(files, func(a, b File) int {
		if c := a.Month.Compare(b.Month); c != 0 {
			return c
		}
		return strings.Compare(a.Group, b.Group)
	})
	return files, unknown
}
