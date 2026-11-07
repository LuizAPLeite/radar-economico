package sgs

import "time"

// Window é um intervalo de datas fechado: inclui Start e End.
type Window struct {
	Start time.Time
	End   time.Time
}

// SplitWindows quebra o período [start, end] em janelas consecutivas de no
// máximo maxYears anos cada, porque a API SGS recusa (HTTP 406) consultas de
// mais de 10 anos em séries diárias.
//
// Regras (os testes em windows_test.go descrevem cada uma):
//   - cada janela vai de Start até Start + maxYears anos - 1 dia
//     (ex.: 07/11/2016 → 06/11/2026, que a API aceita);
//   - a próxima janela começa no dia seguinte ao fim da anterior;
//   - a última janela termina em end (nunca passa dele);
//   - se start for depois de end, ou maxYears <= 0, devolve nil.
func SplitWindows(start, end time.Time, maxYears int) []Window {
	if maxYears <= 0 || start.After(end) {
		return nil
	}

	var windows []Window
	cursor := start

	for !cursor.After(end) {
		windowEnd := cursor.AddDate(maxYears, 0, -1)
		if windowEnd.After(end) {
			windowEnd = end
		}
		windows = append(windows, Window{Start: cursor, End: windowEnd})
		cursor = windowEnd.AddDate(0, 0, 1)
	}

	return windows
}
