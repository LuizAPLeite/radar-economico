// Command collector coleta dados das fontes públicas e grava no schema raw.
// Na Fase 0 é apenas um esqueleto; os coletores chegam na Fase 1.
package main

import (
	"log/slog"
	"os"
)

func main() {
	// slog.NewJSONHandler gera logs estruturados (uma linha JSON por evento),
	// fáceis de filtrar em produção.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("collector iniciado", "fase", 0)
}
