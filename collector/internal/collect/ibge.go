// Coleta do IBGE: a lista de municípios e o centroide de cada um, usados
// para casar os nomes de município da ANP com códigos oficiais e pôr os
// preços no mapa.
package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const ibgeAPI = "https://servicodados.ibge.gov.br/api"

// IBGEDataset é um conjunto de dados do IBGE e o endereço dele.
type IBGEDataset struct {
	Name string // valor gravado em raw.ibge_responses.dataset
	URL  string
}

// IBGEDatasets são os conjuntos usados pelo dashboard (formato conferido em
// 2026-11-07: 5.571 municípios; centroides de 5.570).
var IBGEDatasets = []IBGEDataset{
	{Name: "municipios", URL: ibgeAPI + "/v1/localidades/municipios?view=nivelado"},
	{Name: "centroides", URL: ibgeAPI + "/v3/malhas/paises/BR/metadados?intrarregiao=municipio"},
}

// IBGESaver grava uma resposta do IBGE se ela mudou desde a última coleta.
type IBGESaver interface {
	SaveIBGE(ctx context.Context, runID int64, dataset, url, sha256 string, payload []byte) (saved bool, err error)
}

// IBGE baixa cada conjunto e grava os que mudaram. Uma falha não impede os
// demais; os erros voltam juntos no final.
func IBGE(ctx context.Context, logger *slog.Logger, g Getter, s IBGESaver, runID int64, datasets []IBGEDataset) error {
	var errs []error
	for _, d := range datasets {
		if err := collectIBGEDataset(ctx, logger, g, s, runID, d); err != nil {
			logger.Error("falha na coleta do IBGE", "dataset", d.Name, "err", err)
			errs = append(errs, fmt.Errorf("ibge %s: %w", d.Name, err))
		}
	}
	return errors.Join(errs...)
}

func collectIBGEDataset(ctx context.Context, logger *slog.Logger, g Getter, s IBGESaver, runID int64, d IBGEDataset) error {
	start := time.Now()
	body, err := g.Get(ctx, d.URL)
	if err != nil {
		return err
	}
	// O collector não interpreta os dados, mas confere que veio o que se
	// espera: uma página de erro em HTML não pode entrar no raw como se fosse JSON.
	if !json.Valid(body) {
		return errors.New("a resposta não é JSON")
	}

	saved, err := s.SaveIBGE(ctx, runID, d.Name, d.URL, sha256Hex(body), body)
	if err != nil {
		return err
	}
	logger.Info("ibge coletado", "dataset", d.Name, "bytes", len(body),
		"changed", saved, "took_ms", time.Since(start).Milliseconds())
	return nil
}
