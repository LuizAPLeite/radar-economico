// Package assistant responde perguntas em português sobre os dados da mart:
// um modelo de linguagem (LLM) escreve o SQL, a API confere e executa.
package assistant

import (
	"errors"
	"fmt"
	"strings"
)

// MaxSQLLength limita o tamanho do SQL aceito. As consultas que respondem às
// perguntas do site têm poucas linhas; um texto enorme é sinal de problema.
const MaxSQLLength = 4000

// ErrUnsafeSQL é o erro devolvido para todo SQL recusado. As mensagens
// específicas o embrulham (fmt.Errorf com %w), e quem chama testa com
// errors.Is(err, ErrUnsafeSQL).
var ErrUnsafeSQL = errors.New("SQL recusado")

// CheckSQL é a PRIMEIRA camada de proteção contra um SQL perigoso escrito
// pelo LLM. Não é a única: a consulta ainda roda num papel que só lê
// algumas tabelas (migration 000008), numa transação somente leitura e com
// tempo máximo. Esta camada recusa cedo o que nunca deveria chegar ao banco.
//
// Devolve o SQL limpo (sem espaços nas pontas e sem o ";" final) ou um erro
// que embrulha ErrUnsafeSQL.
//
// Regras, nesta ordem: limpa espaços e um ";" final; recusa vazio ou longo
// demais; mais de um comando; comentários (podem esconder texto do leitor e
// de regras simples como esta); e tudo que não comece por SELECT ou WITH.
func CheckSQL(sql string) (string, error) {
	// Regra 1: limpa as pontas e UM ";" final.
	sql = strings.TrimSpace(sql)
	sql = strings.TrimSuffix(sql, ";")
	sql = strings.TrimSpace(sql)

	// Regra 2: vazio ou longo demais.
	if sql == "" {
		return "", fmt.Errorf("%w: vazio", ErrUnsafeSQL)
	}
	if len(sql) > MaxSQLLength {
		return "", fmt.Errorf("%w: mais de %d caracteres", ErrUnsafeSQL, MaxSQLLength)
	}

	// Regra 3: só um comando. ("SELECT 1;;" cai aqui: a regra 1 tirou só um.)
	if strings.Contains(sql, ";") {
		return "", fmt.Errorf("%w: mais de um comando", ErrUnsafeSQL)
	}

	// Regra 4: sem comentários.
	if strings.Contains(sql, "--") || strings.Contains(sql, "/*") {
		return "", fmt.Errorf("%w: comentários não são aceitos", ErrUnsafeSQL)
	}

	// Regra 5: a primeira palavra é SELECT ou WITH. Fields separa por
	// qualquer espaço (inclusive quebra de linha); o SQL não está vazio, então
	// há pelo menos uma palavra. Cut corta "SELECT(1)" em "SELECT".
	first, _, _ := strings.Cut(strings.Fields(sql)[0], "(")
	if !strings.EqualFold(first, "SELECT") && !strings.EqualFold(first, "WITH") {
		return "", fmt.Errorf("%w: só consultas SELECT ou WITH (veio %q)", ErrUnsafeSQL, first)
	}

	return sql, nil
}
