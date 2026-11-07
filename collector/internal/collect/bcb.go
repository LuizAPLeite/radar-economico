// Coleta do Banco Central (API SGS): monta as consultas, executa em paralelo
// com limite de concorrência e entrega cada resposta para gravação.
package collect

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/LuizAPLeite/radar-economico/collector/internal/sgs"
)

// Series descreve uma série do SGS que queremos coletar.
type Series struct {
	Code  int
	Name  string
	Daily bool // séries diárias exigem datas e janelas de até 10 anos
}

// BCBSeries são as séries do MVP (Fase 1).
var BCBSeries = []Series{
	{Code: 432, Name: "Meta Selic", Daily: true},
	{Code: 433, Name: "IPCA (variação mensal)", Daily: false},
	{Code: 1, Name: "Dólar comercial (venda)", Daily: true},
}

// SGSFetcher busca uma consulta na API SGS.
type SGSFetcher interface {
	Fetch(ctx context.Context, req sgs.Request) (sgs.Response, error)
}

// SGSSaver grava uma resposta bruta da API SGS.
type SGSSaver interface {
	SaveResponse(ctx context.Context, runID int64, resp sgs.Response) error
}

// BuildRequests transforma a lista de séries em consultas: séries diárias
// viram uma consulta por janela de até 10 anos entre start e end; as demais
// viram uma única consulta sem datas (a API devolve o histórico inteiro).
func BuildRequests(series []Series, start, end time.Time) []sgs.Request {
	var reqs []sgs.Request
	for _, s := range series {
		if !s.Daily {
			reqs = append(reqs, sgs.Request{SeriesCode: s.Code})
			continue
		}
		for _, w := range sgs.SplitWindows(start, end, sgs.MaxDailyWindowYears) {
			reqs = append(reqs, sgs.Request{SeriesCode: s.Code, Start: w.Start, End: w.End})
		}
	}
	return reqs
}

// BCBStats resume uma coleta do Banco Central.
type BCBStats struct {
	Saved  int // respostas gravadas (inclui as vazias)
	Empty  int // períodos sem dados
	Failed int // consultas que falharam
}

// BCB executa as consultas com no máximo `concurrency` ao mesmo tempo.
// Uma consulta que falha não interrompe as outras: o objetivo é gravar o
// máximo possível. Todos os erros voltam juntos no final.
func BCB(ctx context.Context, logger *slog.Logger, f SGSFetcher, s SGSSaver,
	runID int64, reqs []sgs.Request, concurrency int,
) (BCBStats, error) {
	// errgroup.Group é um WaitGroup com extras. SetLimit faz g.Go bloquear
	// quando já há `concurrency` goroutines rodando, o que funciona como um
	// semáforo e evita disparar dezenas de requisições de uma vez no BCB.
	var g errgroup.Group
	g.SetLimit(concurrency)

	var (
		mu    sync.Mutex // protege stats e errs, escritos por várias goroutines
		stats BCBStats
		errs  []error
	)

	for _, req := range reqs {
		// Desde o Go 1.22, cada iteração tem sua própria variável `req`,
		// então é seguro usá-la dentro da goroutine.
		g.Go(func() error {
			start := time.Now()
			resp, err := f.Fetch(ctx, req)
			if err == nil {
				err = s.SaveResponse(ctx, runID, resp)
			}

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				stats.Failed++
				errs = append(errs, fmt.Errorf("série %d %s: %w", req.SeriesCode, describe(req), err))
				logger.Error("consulta falhou", "series", req.SeriesCode, "window", describe(req), "err", err)
				return nil // não cancela as demais
			}
			stats.Saved++
			if resp.Empty() {
				stats.Empty++
			}
			logger.Info("resposta gravada",
				"series", req.SeriesCode, "window", describe(req),
				"status", resp.StatusCode, "bytes", len(resp.Body), "took_ms", time.Since(start).Milliseconds())
			return nil
		})
	}

	_ = g.Wait() // as goroutines nunca devolvem erro; eles ficam em errs
	return stats, errors.Join(errs...)
}

func describe(req sgs.Request) string {
	if req.Start.IsZero() {
		return "histórico completo"
	}
	return req.Start.Format(time.DateOnly) + "→" + req.End.Format(time.DateOnly)
}
