package collect

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeGetter devolve, para cada URL, o corpo ou o erro configurado.
type fakeGetter struct {
	bodies map[string]string
	errs   map[string]error
}

func (f *fakeGetter) Get(_ context.Context, url string) ([]byte, error) {
	if err := f.errs[url]; err != nil {
		return nil, err
	}
	return []byte(f.bodies[url]), nil
}

type savedIBGE struct{ dataset, sha string }

// fakeIBGESaver registra o que foi gravado; "mudou" diz o que responder.
type fakeIBGESaver struct {
	changed bool
	saved   []savedIBGE
}

func (f *fakeIBGESaver) SaveIBGE(_ context.Context, _ int64, dataset, _, sha string, _ []byte) (bool, error) {
	f.saved = append(f.saved, savedIBGE{dataset, sha})
	return f.changed, nil
}

var testDatasets = []IBGEDataset{{"municipios", "http://ibge/m"}, {"centroides", "http://ibge/c"}}

func TestIBGE_SavesEveryDataset(t *testing.T) {
	g := &fakeGetter{bodies: map[string]string{"http://ibge/m": `[{"id":1}]`, "http://ibge/c": `[]`}}
	s := &fakeIBGESaver{changed: true}

	if err := IBGE(context.Background(), discard, g, s, 1, testDatasets); err != nil {
		t.Fatal(err)
	}
	if len(s.saved) != 2 || s.saved[0].dataset != "municipios" || s.saved[1].dataset != "centroides" {
		t.Fatalf("gravados = %+v", s.saved)
	}
	// O hash identifica o conteúdo: corpos diferentes, hashes diferentes.
	if s.saved[0].sha == s.saved[1].sha || len(s.saved[0].sha) != 64 {
		t.Errorf("hashes = %q, %q", s.saved[0].sha, s.saved[1].sha)
	}
}

func TestIBGE_UnchangedIsNotAnError(t *testing.T) {
	g := &fakeGetter{bodies: map[string]string{"http://ibge/m": `[]`, "http://ibge/c": `[]`}}
	s := &fakeIBGESaver{changed: false} // o banco já tinha o mesmo conteúdo

	if err := IBGE(context.Background(), discard, g, s, 1, testDatasets); err != nil {
		t.Fatal(err)
	}
}

func TestIBGE_FailureDoesNotStopTheOtherDataset(t *testing.T) {
	g := &fakeGetter{
		bodies: map[string]string{"http://ibge/c": `[]`},
		errs:   map[string]error{"http://ibge/m": errors.New("timeout")},
	}
	s := &fakeIBGESaver{changed: true}

	err := IBGE(context.Background(), discard, g, s, 1, testDatasets)

	if err == nil || !strings.Contains(err.Error(), "municipios") {
		t.Fatalf("esperava erro citando municipios, veio %v", err)
	}
	if len(s.saved) != 1 || s.saved[0].dataset != "centroides" {
		t.Errorf("centroides deveria ter sido gravado mesmo assim: %+v", s.saved)
	}
}

func TestIBGE_RejectsNonJSON(t *testing.T) {
	g := &fakeGetter{bodies: map[string]string{
		"http://ibge/m": `<html>manutenção</html>`,
		"http://ibge/c": `[]`,
	}}
	s := &fakeIBGESaver{changed: true}

	if err := IBGE(context.Background(), discard, g, s, 1, testDatasets); err == nil {
		t.Fatal("esperava erro para resposta em HTML")
	}
	if len(s.saved) != 1 {
		t.Errorf("só o JSON válido deveria ser gravado: %+v", s.saved)
	}
}
