package assistant

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxRows é quantas linhas do resultado voltam no máximo.
const MaxRows = 500

// queryTimeout é o tempo máximo de uma consulta do assistente no Postgres.
const queryTimeout = "3s"

// Column descreve uma coluna do resultado.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"` // "date", "number" ou "text"
}

// Result é o resultado de uma consulta, pronto para virar JSON: datas como
// AAAA-MM-DD, números como float64, o resto como texto; NULL vira nil.
type Result struct {
	Columns   []Column `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"` // havia mais que MaxRows linhas
}

// Executor roda uma consulta já conferida por CheckSQL.
type Executor interface {
	Run(ctx context.Context, sql string) (Result, error)
}

// PgExecutor roda as consultas no Postgres com todas as travas:
//   - transação somente leitura;
//   - dentro do papel radar_assistant (SET LOCAL ROLE), que só lê algumas
//     tabelas da mart;
//   - statement_timeout de 3 segundos;
//   - no máximo MaxRows linhas (a consulta vira subconsulta com LIMIT).
//
// SET LOCAL vale só até o fim da transação: quando a conexão volta ao pool,
// ela é de novo a da API, sem o papel e sem o timeout.
type PgExecutor struct {
	Pool *pgxpool.Pool
}

var _ Executor = (*PgExecutor)(nil)

func (e *PgExecutor) Run(ctx context.Context, sql string) (Result, error) {
	tx, err := e.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Result{}, fmt.Errorf("assistente: abrindo a transação: %w", err)
	}
	// Só leitura: não há o que confirmar. Rollback devolve a conexão limpa.
	defer func() { _ = tx.Rollback(context.Background()) }()

	for _, stmt := range []string{
		"SET LOCAL ROLE radar_assistant",
		"SET LOCAL statement_timeout = '" + queryTimeout + "'",
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return Result{}, fmt.Errorf("assistente: %s: %w", stmt, err)
		}
	}

	// A consulta do LLM vira subconsulta: o LIMIT vale qualquer que seja o
	// SQL (inclusive um WITH). Uma linha a mais diz se houve corte.
	wrapped := fmt.Sprintf("SELECT * FROM (\n%s\n) AS resposta LIMIT %d", sql, MaxRows+1)
	rows, err := tx.Query(ctx, wrapped)
	if err != nil {
		return Result{}, fmt.Errorf("assistente: consulta: %w", err)
	}
	defer rows.Close()

	var res Result
	for _, f := range rows.FieldDescriptions() {
		res.Columns = append(res.Columns, Column{Name: f.Name, Type: columnType(f.DataTypeOID)})
	}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return Result{}, fmt.Errorf("assistente: lendo o resultado: %w", err)
		}
		if len(res.Rows) == MaxRows {
			res.Truncated = true
			break
		}
		row := make([]any, len(values))
		for i, v := range values {
			row[i] = jsonValue(v)
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("assistente: consulta: %w", err)
	}
	if res.Rows == nil {
		res.Rows = [][]any{} // [] no JSON, não null
	}
	return res, nil
}

// columnType classifica o tipo do Postgres (pelo OID) para o front.
func columnType(oid uint32) string {
	switch oid {
	case pgtype.DateOID, pgtype.TimestampOID, pgtype.TimestamptzOID:
		return "date"
	case pgtype.NumericOID, pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID,
		pgtype.Float4OID, pgtype.Float8OID:
		return "number"
	default:
		return "text"
	}
}

// jsonValue converte o valor que o pgx devolve num que vira JSON simples.
func jsonValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return x.Format(time.DateOnly)
	case pgtype.Numeric:
		f, err := x.Float64Value()
		if err != nil || !f.Valid {
			return nil
		}
		return f.Float64
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case float32:
		return float64(x)
	case float64, string, bool:
		return x
	default:
		return fmt.Sprint(x)
	}
}
