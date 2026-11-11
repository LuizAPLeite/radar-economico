// Coleta de fontes que publicam um arquivo inteiro (planilha, CSV): baixa,
// confere o formato e grava em raw.source_files só quando o conteúdo mudou.
package collect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// RemoteFile é um arquivo publicado por uma fonte.
type RemoteFile struct {
	Name string // valor gravado em raw.source_files.name
	URL  string
	// Validate confere que o corpo tem o formato esperado, para uma página
	// de erro em HTML não entrar no raw como se fosse o arquivo.
	Validate func(body []byte) error
}

// FileSaver grava um arquivo compactado se ele mudou desde a última versão
// guardada (mesma fonte e nome). Devolve se gravou.
type FileSaver interface {
	SaveSourceFile(ctx context.Context, runID int64, source, name, url, sha256 string, size int64, gz []byte) (saved bool, err error)
}

// FilesStats resume uma coleta de arquivos.
type FilesStats struct {
	Saved, Unchanged, Failed int
}

// Files baixa cada arquivo e grava os que mudaram. Uma falha não impede os
// demais; os erros voltam juntos no final.
func Files(ctx context.Context, logger *slog.Logger, g Getter, s FileSaver, runID int64, source string, files []RemoteFile) (FilesStats, error) {
	var (
		stats FilesStats
		errs  []error
	)
	for _, f := range files {
		saved, err := collectFile(ctx, logger, g, s, runID, source, f)
		switch {
		case err != nil:
			stats.Failed++
			logger.Error("falha ao coletar arquivo", "source", source, "name", f.Name, "err", err)
			errs = append(errs, fmt.Errorf("%s %s: %w", source, f.Name, err))
		case saved:
			stats.Saved++
		default:
			stats.Unchanged++
		}
	}
	return stats, errors.Join(errs...)
}

func collectFile(ctx context.Context, logger *slog.Logger, g Getter, s FileSaver, runID int64, source string, f RemoteFile) (bool, error) {
	start := time.Now()
	body, err := g.Get(ctx, f.URL)
	if err != nil {
		return false, err
	}
	if err := f.Validate(body); err != nil {
		return false, err
	}
	gz, err := gzipBytes(body)
	if err != nil {
		return false, err
	}
	saved, err := s.SaveSourceFile(ctx, runID, source, f.Name, f.URL, sha256Hex(body), int64(len(body)), gz)
	if err != nil {
		return false, err
	}
	logger.Info("arquivo coletado", "source", source, "name", f.Name, "bytes", len(body),
		"changed", saved, "took_ms", time.Since(start).Milliseconds())
	return saved, nil
}

// anpSeriesURL é a pasta da série histórica do levantamento de preços (SHLP).
const anpSeriesURL = "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/precos-revenda-e-de-distribuicao-combustiveis/shlp"

// ANPSeriesFiles são as planilhas da média semanal no Brasil calculada pela
// própria ANP (layout conferido em 2026-11-11: cabeçalho na linha 20 da
// atual e na 15 da antiga; semanas de domingo a sábado). A de 2004 a 2012
// não muda mais: o SHA-256 evita gravá-la de novo.
var ANPSeriesFiles = []RemoteFile{
	{Name: "semanal-brasil-2004-a-2012", URL: anpSeriesURL + "/2001-2012/semanal-brasil-2004-a-2012.xlsx", Validate: validateXLSX},
	{Name: "semanal-brasil-desde-2013", URL: anpSeriesURL + "/semanal/semanal-brasil-desde-2013.xlsx", Validate: validateXLSX},
}

// zipMagic são os primeiros bytes de todo arquivo ZIP. Um .xlsx é um ZIP
// de arquivos XML.
var zipMagic = []byte("PK\x03\x04")

func validateXLSX(body []byte) error {
	if !bytes.HasPrefix(body, zipMagic) {
		return errors.New("o arquivo não é uma planilha xlsx")
	}
	return nil
}
