// Package postgres cria o pool de conexões da API com o banco.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect abre um pool de conexões SOMENTE LEITURA e confirma que o banco
// responde.
//
// default_transaction_read_only=on faz o próprio Postgres recusar qualquer
// INSERT, UPDATE, DELETE ou DDL feito por estas conexões. A regra de
// arquitetura "a API só lê a mart" deixa de ser só uma convenção.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: DATABASE_URL inválida: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"

	// O Neon (produção) desliga o banco após 5 min sem consultas e derruba as
	// conexões abertas. Fechando as ociosas antes disso, a próxima requisição
	// abre uma conexão nova, em vez de tropeçar numa já derrubada.
	cfg.MaxConnIdleTime = time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: criando pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: conectando: %w", err)
	}
	return pool, nil
}
