package anp

// ParseFileURL identifica o grupo de produtos e o mês de referência de um
// arquivo mensal pelo endereço dele. Devolve ok = false para o que não for um
// arquivo mensal de gasolina/etanol ou de diesel/GNV (por exemplo, GLP).
//
// Exemplo:
//
//	".../shpc/dsan/2026/10-gasolina-etanol.csv"
//	  → File{Group: "gasolina-etanol", Month: 01/10/2026, URL: <o próprio endereço>}, true
//
// TODO(LuizAPLeite): implementar. Os casos de teste em parse_test.go são os
// nomes reais publicados pela ANP. Passos:
//
//  1. Ano: são os 4 dígitos logo depois de "/dsan/". strings.Cut divide um
//     texto em "antes" e "depois" de um separador; strconv.Atoi converte
//     texto em número.
//  2. Nome do arquivo: o que vem depois da última "/", sem o ".csv"
//     (path.Base e strings.TrimSuffix ajudam).
//  3. Grupo: o nome contém GroupGasolinaEtanol ou GroupDieselGNV?
//     (strings.Contains). Se não contém nenhum dos dois, devolva false.
//  4. Mês: os nomes de 2026 COMEÇAM com o mês ("10-gasolina-etanol");
//     os de 2023 a 2025 TERMINAM com ele ("precos-gasolina-etanol-10").
//     Tente os dois jeitos e confira que o número fica entre 1 e 12.
//  5. Monte o File com Month = time.Date(ano, time.Month(mês), 1, 0, 0, 0, 0, time.UTC).
func ParseFileURL(url string) (File, bool) {
	return File{}, false
}
