package assistant

import (
	"strings"
	"testing"
)

// O log não guarda o texto da pergunta (LGPD), só tamanho e hash.
func TestQuestionRef_HidesText(t *testing.T) {
	ref := questionRef("Meu CPF é 123.456.789-00, qual a Selic?")
	if strings.Contains(ref, "CPF") || strings.Contains(ref, "123") {
		t.Fatalf("o texto vazou no log: %q", ref)
	}
	if !strings.HasPrefix(ref, "39 caracteres, sha256 ") {
		t.Fatalf("formato inesperado: %q", ref)
	}
	if ref != questionRef("Meu CPF é 123.456.789-00, qual a Selic?") {
		t.Fatal("a mesma pergunta deve dar a mesma referência")
	}
}
