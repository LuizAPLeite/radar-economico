// Package assistant responde perguntas em português sobre os dados da mart:
// um modelo de linguagem (LLM) escreve o SQL, a API confere e executa.
package assistant

import (
	"errors"
	"fmt"
	"regexp"
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
// de regras simples como esta); tudo que não comece por SELECT ou WITH; e
// chamadas a funções perigosas.
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

	// Regra 6: sem funções que mudam a sessão, executam outro SQL ou mexem no
	// servidor. Defesa em profundidade: a proteção principal é a API conectar
	// como radar_api (migration 000009), sem nada a ganhar com elas.
	if m := dangerousCall.FindString(sql); m != "" {
		return "", fmt.Errorf("%w: função não permitida (%s)", ErrUnsafeSQL, strings.TrimSpace(m))
	}
	if unicodeEscape.MatchString(sql) {
		return "", fmt.Errorf("%w: identificadores com escape Unicode (U&) não são aceitos", ErrUnsafeSQL)
	}

	return sql, nil
}

// dangerousCall acha chamadas a funções que não têm lugar numa consulta de
// dados, com ou sem aspas no nome e com ou sem o esquema na frente
// (pg_catalog.set_config). Foi uma delas, set_config('role', ...), junto com
// query_to_xml, que abriu o banco inteiro no achado AM-01.
var dangerousCall = regexp.MustCompile(`(?i)\b"?(` +
	`set_config|` + // muda configurações da sessão, inclusive o papel
	`(query|table|cursor|schema|database)_to_xml\w*|` + // executam outro SQL
	`dblink\w*|` + // conexão a outro banco
	`pg_read_\w+|pg_ls_\w+|pg_stat_file|lo_\w+|` + // arquivos do servidor
	`pg_sleep\w*|pg_terminate_backend|pg_cancel_backend|pg_reload_conf|` + // servidor
	`pg_advisory_\w+|nextval|setval` + // travas e sequências
	`)"?\s*\(`)

// unicodeEscape acha U&"..." e U&'...': escrevem nomes com escapes, um jeito
// de esconder um nome de função das regras acima.
var unicodeEscape = regexp.MustCompile(`(?i)\bU&["']`)
