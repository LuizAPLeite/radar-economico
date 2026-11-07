package collect

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LuizAPLeite/radar-economico/collector/internal/anp"
)

const csvHeader = "\xef\xbb\xbfRegiao - Sigla;Estado - Sigla;Municipio;Revenda;CNPJ da Revenda;Nome da Rua;Numero Rua;Complemento;Bairro;Cep;Produto;Data da Coleta;Valor de Venda;Valor de Compra;Unidade de Medida;Bandeira\r\n"

func mon(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }

func anpFile(group string, m time.Time) anp.File {
	return anp.File{Group: group, Month: m, URL: "http://anp/" + group + "/" + m.Format("2006-01") + ".csv"}
}

func csvBody(f anp.File) string {
	return csvHeader + "SE;SP;GUARULHOS;POSTO;x;x;1;;x;x;GASOLINA;07/" + f.Month.Format("01/2006") + ";6,79;;R$ / litro;X\r\n"
}

// safeGetter é como fakeGetter, mas aguenta ser chamado de várias goroutines.
type safeGetter struct {
	mu     sync.Mutex
	bodies map[string]string
	calls  []string
}

func (g *safeGetter) Get(_ context.Context, url string) ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, url)
	return []byte(g.bodies[url]), nil
}

type savedFile struct {
	key     anp.Key
	content string
}

type fakeANPStore struct {
	mu     sync.Mutex
	latest map[anp.Key]string
	saved  []savedFile
}

func (s *fakeANPStore) LatestANPHashes(context.Context) (map[anp.Key]string, error) {
	return s.latest, nil
}

func (s *fakeANPStore) SaveANPFile(_ context.Context, _ int64, f anp.File, _ string, _ int64, gz []byte) error {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return err
	}
	content, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, savedFile{f.Key(), string(content)})
	return nil
}

// cenário: ago, set e out/2026 para os dois grupos, menos diesel-gnv de setembro.
func scenario() ([]anp.File, *safeGetter) {
	files := []anp.File{
		anpFile(anp.GroupGasolinaEtanol, mon(2026, time.August)),
		anpFile(anp.GroupDieselGNV, mon(2026, time.August)),
		anpFile(anp.GroupGasolinaEtanol, mon(2026, time.September)),
		anpFile(anp.GroupGasolinaEtanol, mon(2026, time.October)),
		anpFile(anp.GroupDieselGNV, mon(2026, time.October)),
	}
	g := &safeGetter{bodies: map[string]string{"http://anp/pagina": "<html>"}}
	for _, f := range files {
		g.bodies[f.URL] = csvBody(f)
	}
	return files, g
}

func testANPConfig(files []anp.File) ANPConfig {
	return ANPConfig{
		PageURL:     "http://anp/pagina",
		StartMonth:  mon(2026, time.August),
		Recheck:     1,
		Concurrency: 2,
		Discover:    func([]byte) ([]anp.File, []string) { return files, []string{"glp"} },
	}
}

func TestANP_FirstRunSavesEverythingCompressed(t *testing.T) {
	files, g := scenario()
	s := &fakeANPStore{latest: map[anp.Key]string{}}

	stats, err := ANP(context.Background(), discard, g, s, 1, testANPConfig(files))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Found != 5 || stats.Saved != 5 || stats.Unchanged != 0 || stats.Failed != 0 {
		t.Errorf("stats = %+v", stats)
	}
	// O que foi gravado, descompactado, é idêntico ao que foi baixado.
	for _, sv := range s.saved {
		f := anpFile(sv.key.Group, mustMonth(t, sv.key.Month))
		if sv.content != csvBody(f) {
			t.Errorf("%+v: conteúdo gravado difere do baixado", sv.key)
		}
	}
	if !slices.Equal(stats.Missing, []string{"diesel-gnv 2026-09"}) {
		t.Errorf("Missing = %v", stats.Missing)
	}
}

func TestANP_IncrementalRun(t *testing.T) {
	files, g := scenario()
	// Tudo já guardado; outubro de gasolina-etanol com o mesmo conteúdo.
	latest := map[anp.Key]string{}
	for _, f := range files {
		latest[f.Key()] = "hash-antigo"
	}
	octGas := anpFile(anp.GroupGasolinaEtanol, mon(2026, time.October))
	latest[octGas.Key()] = sha256Hex([]byte(csvBody(octGas)))
	s := &fakeANPStore{latest: latest}

	stats, err := ANP(context.Background(), discard, g, s, 1, testANPConfig(files))
	if err != nil {
		t.Fatal(err)
	}

	// Com Recheck = 1, só outubro (o mês mais recente) é baixado de novo:
	// gasolina-etanol não mudou; diesel-gnv mudou e é gravado.
	if stats.Downloaded != 2 || stats.Unchanged != 1 || stats.Saved != 1 {
		t.Errorf("stats = %+v", stats)
	}
	for _, url := range g.calls {
		if strings.Contains(url, "2026-08") || strings.Contains(url, "2026-09") {
			t.Errorf("não deveria baixar de novo um mês antigo já guardado: %s", url)
		}
	}
}

func TestANP_StartMonthFiltersOldFiles(t *testing.T) {
	files, g := scenario()
	s := &fakeANPStore{latest: map[anp.Key]string{}}
	cfg := testANPConfig(files)
	cfg.StartMonth = mon(2026, time.October)

	stats, err := ANP(context.Background(), discard, g, s, 1, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Found != 2 || len(stats.Missing) != 0 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestANP_RejectsHTMLInsteadOfCSV(t *testing.T) {
	files, g := scenario()
	g.bodies[files[0].URL] = "<html><body>Manutenção</body></html>"
	s := &fakeANPStore{latest: map[anp.Key]string{}}

	stats, err := ANP(context.Background(), discard, g, s, 1, testANPConfig(files))

	if err == nil || stats.Failed != 1 || stats.Saved != 4 {
		t.Errorf("err = %v, stats = %+v", err, stats)
	}
}

func TestANP_NoRecognizedFilesIsAnError(t *testing.T) {
	_, g := scenario()
	s := &fakeANPStore{latest: map[anp.Key]string{}}
	cfg := testANPConfig(nil)

	if _, err := ANP(context.Background(), discard, g, s, 1, cfg); err == nil {
		t.Fatal("esperava erro quando a página não tem nenhum arquivo reconhecido")
	}
}

func TestLooksLikeCSV(t *testing.T) {
	for body, want := range map[string]bool{
		csvHeader:                   true,
		"<!DOCTYPE html><html>":     false,
		"":                          false,
		"um;dois\n1;2":              false, // poucas colunas
		"\xef\xbb\xbfa;b;c;d;e;f\n": true,
	} {
		if got := looksLikeCSV([]byte(body)); got != want {
			t.Errorf("looksLikeCSV(%q) = %v, quer %v", body, got, want)
		}
	}
}

func mustMonth(t *testing.T, s string) time.Time {
	t.Helper()
	m, err := time.Parse("2006-01", s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
