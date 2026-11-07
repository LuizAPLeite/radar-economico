// Coleta da ANP: descobre os arquivos mensais na página de dados abertos,
// baixa os que ainda não estão no raw (e os meses mais recentes, que a ANP
// pode republicar corrigidos) e grava o conteúdo intacto, compactado.
package collect

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/LuizAPLeite/radar-economico/collector/internal/anp"
)

// ANPStore é o que a coleta da ANP precisa do banco.
type ANPStore interface {
	// LatestANPHashes devolve o SHA-256 da versão mais recente guardada de
	// cada arquivo (grupo e mês).
	LatestANPHashes(ctx context.Context) (map[anp.Key]string, error)
	SaveANPFile(ctx context.Context, runID int64, f anp.File, sha256 string, size int64, gz []byte) error
}

// ANPConfig ajusta a coleta.
type ANPConfig struct {
	PageURL     string
	StartMonth  time.Time // primeiro mês coletado (1º dia, UTC)
	Recheck     int       // meses mais recentes baixados de novo mesmo se já guardados
	Concurrency int       // downloads simultâneos: o servidor da ANP é lento
	// Discover identifica os arquivos no HTML da página (anp.Discover).
	// É um campo para os testes poderem trocar por uma versão falsa.
	Discover func(html []byte) (files []anp.File, unknown []string)
}

// ANPStats resume uma coleta da ANP.
type ANPStats struct {
	Found      int      // arquivos reconhecidos na janela de meses
	Downloaded int      // arquivos baixados
	Saved      int      // gravados (novos ou alterados)
	Unchanged  int      // baixados, mas iguais à versão guardada
	Failed     int      // downloads ou gravações que falharam
	Missing    []string // "grupo AAAA-MM" sem arquivo publicado
}

// ANP executa a coleta. Uma falha num arquivo não interrompe os outros; os
// erros voltam juntos no final.
func ANP(ctx context.Context, logger *slog.Logger, g Getter, s ANPStore, runID int64, cfg ANPConfig) (ANPStats, error) {
	var stats ANPStats

	page, err := g.Get(ctx, cfg.PageURL)
	if err != nil {
		return stats, fmt.Errorf("página da ANP: %w", err)
	}
	files, unknown := cfg.Discover(page)
	logger.Info("página da ANP lida", "recognized", len(files), "ignored", len(unknown))

	files = filterANPFiles(logger, files, cfg.StartMonth)
	if len(files) == 0 {
		// A página sempre tem arquivos: se nenhum foi reconhecido, o formato
		// mudou e o código precisa de ajuste. Melhor falhar do que ficar quieto.
		return stats, errors.New("nenhum arquivo mensal reconhecido na página da ANP: o formato pode ter mudado")
	}
	stats.Found = len(files)

	stats.Missing = missingMonths(files, cfg.StartMonth)
	for _, m := range stats.Missing {
		logger.Warn("mês sem arquivo publicado pela ANP", "file", m)
	}

	latest, err := s.LatestANPHashes(ctx)
	if err != nil {
		return stats, err
	}
	toFetch := selectForDownload(files, latest, cfg.Recheck)

	var (
		group errgroup.Group
		mu    sync.Mutex
		errs  []error
	)
	group.SetLimit(cfg.Concurrency)
	for _, f := range toFetch {
		group.Go(func() error {
			saved, err := fetchANPFile(ctx, logger, g, s, runID, f, latest[f.Key()])

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				stats.Failed++
				errs = append(errs, fmt.Errorf("anp %s %s: %w", f.Group, f.Key().Month, err))
				logger.Error("falha num arquivo da ANP", "group", f.Group, "month", f.Key().Month, "err", err)
			case saved:
				stats.Downloaded++
				stats.Saved++
			default:
				stats.Downloaded++
				stats.Unchanged++
			}
			return nil // os erros ficam em errs: não cancelar os outros downloads
		})
	}
	_ = group.Wait()
	return stats, errors.Join(errs...)
}

// filterANPFiles descarta os meses anteriores a start e os arquivos repetidos
// (mesmo grupo e mês em dois links), mantendo o primeiro.
func filterANPFiles(logger *slog.Logger, files []anp.File, start time.Time) []anp.File {
	var out []anp.File
	seen := make(map[anp.Key]bool)
	for _, f := range files {
		if f.Month.Before(start) {
			continue
		}
		if seen[f.Key()] {
			logger.Warn("arquivo repetido na página da ANP; mantendo o primeiro", "group", f.Group, "month", f.Key().Month, "url", f.URL)
			continue
		}
		seen[f.Key()] = true
		out = append(out, f)
	}
	return out
}

// selectForDownload escolhe o que baixar: os arquivos ainda não guardados e
// os dos `recheck` meses mais recentes (que a ANP pode republicar corrigidos).
// Meses antigos já guardados não são baixados de novo.
func selectForDownload(files []anp.File, latest map[anp.Key]string, recheck int) []anp.File {
	var months []time.Time
	for _, f := range files {
		if !slices.ContainsFunc(months, f.Month.Equal) {
			months = append(months, f.Month)
		}
	}
	slices.SortFunc(months, func(a, b time.Time) int { return b.Compare(a) }) // mais recente primeiro
	recent := months[:min(recheck, len(months))]

	var out []anp.File
	for _, f := range files {
		_, stored := latest[f.Key()]
		if !stored || slices.ContainsFunc(recent, f.Month.Equal) {
			out = append(out, f)
		}
	}
	return out
}

// missingMonths lista, para cada grupo, os meses entre start e o mês mais
// recente publicado que não têm arquivo.
func missingMonths(files []anp.File, start time.Time) []string {
	if len(files) == 0 {
		return nil
	}
	last := files[0].Month
	have := make(map[anp.Key]bool)
	for _, f := range files {
		have[f.Key()] = true
		if f.Month.After(last) {
			last = f.Month
		}
	}
	var missing []string
	for m := start; !m.After(last); m = m.AddDate(0, 1, 0) {
		for _, group := range anp.Groups {
			k := anp.File{Group: group, Month: m}.Key()
			if !have[k] {
				missing = append(missing, k.Group+" "+k.Month)
			}
		}
	}
	return missing
}

// fetchANPFile baixa um arquivo e o grava se mudou. Devolve se gravou.
func fetchANPFile(ctx context.Context, logger *slog.Logger, g Getter, s ANPStore,
	runID int64, f anp.File, previousSHA string,
) (bool, error) {
	start := time.Now()
	body, err := g.Get(ctx, f.URL)
	if err != nil {
		return false, err
	}
	// O collector não interpreta o CSV, mas confere que veio um CSV, e não
	// uma página de erro em HTML que entraria no raw como se fosse dado.
	if !looksLikeCSV(body) {
		return false, errors.New("o conteúdo não parece um CSV da ANP (página de erro?)")
	}

	sha := sha256Hex(body)
	if sha == previousSHA {
		logger.Info("arquivo da ANP sem mudanças", "group", f.Group, "month", f.Key().Month)
		return false, nil
	}

	gz, err := gzipBytes(body)
	if err != nil {
		return false, err
	}
	if err := s.SaveANPFile(ctx, runID, f, sha, int64(len(body)), gz); err != nil {
		return false, err
	}
	logger.Info("arquivo da ANP gravado", "group", f.Group, "month", f.Key().Month,
		"bytes", len(body), "gzip_bytes", len(gz), "took_ms", time.Since(start).Milliseconds())
	return true, nil
}

// looksLikeCSV confere a primeira linha: não começa com "<" (HTML) e tem
// várias colunas separadas por ";" (o cabeçalho da ANP tem 16).
func looksLikeCSV(b []byte) bool {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) // BOM do UTF-8, que a ANP usa
	line, _, _ := bytes.Cut(b, []byte("\n"))
	return len(line) > 0 && line[0] != '<' && bytes.Count(line, []byte(";")) >= 5
}

// gzipBytes compacta b. CSV é texto repetitivo: fica ~4x menor.
func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	// Close escreve o final do formato gzip: sem ele o arquivo fica truncado.
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
