// Package collect orquestra a coleta de cada fonte (Banco Central, IBGE,
// ANP): decide o que buscar, busca e entrega o conteúdo bruto para gravação.
//
// As dependências de rede e de banco entram por interfaces pequenas,
// definidas por quem USA (este pacote), e não por quem implementa: é o padrão
// em Go. Assim os testes trocam a internet e o Postgres por versões falsas,
// sem nenhuma biblioteca de mock.
package collect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// Getter baixa uma URL e devolve o corpo (implementado por fetch.Client).
type Getter interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// sha256Hex devolve o SHA-256 de b em hexadecimal: a "impressão digital" do
// conteúdo, usada para não gravar de novo o que não mudou.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
