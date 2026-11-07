package sgs

import (
	"slices"
	"testing"
	"time"
)

// d cria uma data à meia-noite UTC, para os testes ficarem legíveis.
func d(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestSplitWindows(t *testing.T) {
	tests := []struct {
		name     string
		start    time.Time
		end      time.Time
		maxYears int
		want     []Window
	}{
		{
			name:     "período menor que o máximo vira uma janela",
			start:    d(2024, 1, 1),
			end:      d(2026, 11, 6),
			maxYears: 10,
			want:     []Window{{d(2024, 1, 1), d(2026, 11, 6)}},
		},
		{
			name:     "exatamente 10 anos menos um dia cabe em uma janela",
			start:    d(2016, 11, 7),
			end:      d(2026, 11, 6),
			maxYears: 10,
			want:     []Window{{d(2016, 11, 7), d(2026, 11, 6)}},
		},
		{
			name:     "um dia a mais já abre uma segunda janela",
			start:    d(2016, 11, 7),
			end:      d(2026, 11, 7),
			maxYears: 10,
			want: []Window{
				{d(2016, 11, 7), d(2026, 11, 6)},
				{d(2026, 11, 7), d(2026, 11, 7)},
			},
		},
		{
			name:     "histórico desde 2000 vira três janelas",
			start:    d(2000, 1, 1),
			end:      d(2026, 11, 6),
			maxYears: 10,
			want: []Window{
				{d(2000, 1, 1), d(2009, 12, 31)},
				{d(2010, 1, 1), d(2019, 12, 31)},
				{d(2020, 1, 1), d(2026, 11, 6)},
			},
		},
		{
			name:     "29 de fevereiro",
			start:    d(2016, 2, 29),
			end:      d(2018, 1, 1),
			maxYears: 1,
			want: []Window{
				{d(2016, 2, 29), d(2017, 2, 28)},
				{d(2017, 3, 1), d(2018, 1, 1)},
			},
		},
		{
			name:     "início igual ao fim",
			start:    d(2026, 11, 6),
			end:      d(2026, 11, 6),
			maxYears: 10,
			want:     []Window{{d(2026, 11, 6), d(2026, 11, 6)}},
		},
		{
			name:     "início depois do fim",
			start:    d(2026, 11, 7),
			end:      d(2026, 11, 6),
			maxYears: 10,
			want:     nil,
		},
		{
			name:     "maxYears inválido",
			start:    d(2020, 1, 1),
			end:      d(2026, 11, 6),
			maxYears: 0,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitWindows(tt.start, tt.end, tt.maxYears)

			// slices.EqualFunc compara elemento a elemento. Usamos Time.Equal
			// em vez de ==, que é o jeito certo de comparar instantes em Go.
			equal := slices.EqualFunc(got, tt.want, func(a, b Window) bool {
				return a.Start.Equal(b.Start) && a.End.Equal(b.End)
			})
			if !equal {
				t.Errorf("SplitWindows(%s, %s, %d)\n got: %v\nwant: %v",
					tt.start.Format(time.DateOnly), tt.end.Format(time.DateOnly), tt.maxYears,
					fmtWindows(got), fmtWindows(tt.want))
			}
		})
	}
}

func fmtWindows(ws []Window) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Start.Format(time.DateOnly)+"→"+w.End.Format(time.DateOnly))
	}
	return out
}
