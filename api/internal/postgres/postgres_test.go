package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Teste de integração: sem TEST_DATABASE_URL é pulado.
func TestConnect_IsReadOnly(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definida; pulando teste de integração")
	}
	ctx := context.Background()

	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Leitura funciona.
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM mart.indicators").Scan(&n); err != nil {
		t.Fatalf("leitura falhou: %v", err)
	}

	// Escrita é recusada pelo Postgres antes de tocar em qualquer linha
	// (o WHERE false garante que nada seria apagado mesmo se passasse).
	_, err = pool.Exec(ctx, "DELETE FROM mart.indicator_values WHERE false")

	// errors.As procura, na cadeia de erros, um erro do tipo pedido.
	// O pgx devolve *pgconn.PgError com o código SQLSTATE do Postgres.
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "25006" { // read_only_sql_transaction
		t.Fatalf("esperava erro 25006 (somente leitura), veio: %v", err)
	}
}

func TestConnect_InvalidURL(t *testing.T) {
	if _, err := Connect(context.Background(), "não é uma url://"); err == nil {
		t.Fatal("esperava erro para URL inválida")
	}
}
