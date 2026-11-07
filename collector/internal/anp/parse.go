package anp

import (
	"path"
	"strconv"
	"strings"
	"time"
)

// ParseFileURL identifica o grupo de produtos e o mês de referência de um
// arquivo mensal pelo endereço dele. Devolve ok = false para o que não for um
// arquivo mensal de gasolina/etanol ou de diesel/GNV (por exemplo, GLP).
//
// Exemplo:
//
//	".../shpc/dsan/2026/10-gasolina-etanol.csv"
//	  → File{Group: "gasolina-etanol", Month: 01/10/2026, URL: <o próprio endereço>}, true
func ParseFileURL(url string) (File, bool) {
	// Ano: os 4 dígitos logo depois de "/dsan/".
	// strings.Cut divide o texto em "antes" e "depois" do separador.
	_, after, found := strings.Cut(url, "/dsan/")
	if !found || len(after) < 5 {
		return File{}, false
	}
	year, err := strconv.Atoi(after[:4])
	if err != nil {
		return File{}, false
	}

	// Nome do arquivo sem a extensão: "10-gasolina-etanol".
	name := strings.TrimSuffix(path.Base(url), ".csv")

	// Grupo. Um switch sem expressão funciona como if / else if.
	var group string
	switch {
	case strings.Contains(name, GroupGasolinaEtanol):
		group = GroupGasolinaEtanol
	case strings.Contains(name, GroupDieselGNV):
		group = GroupDieselGNV
	default:
		return File{}, false // GLP ou outro arquivo que não queremos
	}

	// Mês: no começo do nome (2026: "10-gasolina-etanol") ou no fim
	// (2023 a 2025: "precos-gasolina-etanol-10"). O erro do Atoi é descartado
	// (_): se não for número, month fica 0 e a checagem final rejeita.
	month := 0
	if len(name) > 3 && name[2] == '-' {
		month, _ = strconv.Atoi(name[:2])
	}
	if month == 0 && len(name) > 3 && name[len(name)-3] == '-' {
		month, _ = strconv.Atoi(name[len(name)-2:])
	}
	if month < 1 || month > 12 {
		return File{}, false
	}

	return File{
		Group: group,
		Month: time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC),
		URL:   url,
	}, true
}
