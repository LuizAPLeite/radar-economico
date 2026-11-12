// Coleta do FRED (Federal Reserve Bank of St. Louis): o preço diário do
// petróleo Brent em US$, usado na análise de repasse para a bomba.
package collect

import (
	"bytes"
	"errors"
	"fmt"
)

// fredCSVURL devolve o endereço do CSV de uma série do FRED. Não precisa de
// chave de API (formato conferido em 2026-11-11).
func fredCSVURL(seriesID string) string {
	return "https://fred.stlouisfed.org/graph/fredgraph.csv?id=" + seriesID
}

// FREDFiles são as séries do FRED coletadas. DCOILBRENTEU: Brent, US$ por
// barril, diário (dado da EIA, o órgão de energia dos EUA).
var FREDFiles = []RemoteFile{
	{Name: "DCOILBRENTEU", URL: fredCSVURL("DCOILBRENTEU"), Validate: validateFREDCSV("DCOILBRENTEU")},
}

// validateFREDCSV devolve uma função que confere se o corpo é o CSV da
// série seriesID. O CSV do FRED começa assim:
//
//	observation_date,DCOILBRENTEU
//	1987-05-20,18.63
//	1987-05-21,18.45
//
// A função devolvida é uma closure: ela "lembra" o seriesID com que foi
// criada, então o mesmo código valida qualquer série.
func validateFREDCSV(seriesID string) func(body []byte) error {
	want := "observation_date," + seriesID
	return func(body []byte) error {
		// Cut separa na primeira quebra de linha: header antes, rest depois.
		header, rest, _ := bytes.Cut(body, []byte("\n"))
		header = bytes.TrimSuffix(header, []byte("\r")) // quebra de linha do Windows
		if string(header) != want {
			return fmt.Errorf("cabeçalho inesperado no CSV do FRED: %q (esperado %q)", header, want)
		}
		if len(bytes.TrimSpace(rest)) == 0 {
			return errors.New("o CSV do FRED não tem dados")
		}
		return nil
	}
}
