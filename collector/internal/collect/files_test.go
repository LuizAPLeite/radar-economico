package collect

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type savedSourceFile struct {
	source, name, sha string
	size              int64
	content           string // descompactado, para conferir o gzip
}

// fakeFileSaver registra o que foi gravado; unchanged lista os nomes cujo
// conteúdo o "banco" já tem.
type fakeFileSaver struct {
	unchanged map[string]bool
	saved     []savedSourceFile
}

func (f *fakeFileSaver) SaveSourceFile(_ context.Context, _ int64, source, name, _, sha string, size int64, gz []byte) (bool, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return false, err
	}
	content, err := io.ReadAll(zr)
	if err != nil {
		return false, err
	}
	if f.unchanged[name] {
		return false, nil
	}
	f.saved = append(f.saved, savedSourceFile{source, name, sha, size, string(content)})
	return true, nil
}

func acceptAll([]byte) error { return nil }

func TestFiles_SavesChangedAndSkipsUnchanged(t *testing.T) {
	files := []RemoteFile{
		{Name: "a", URL: "http://x/a", Validate: acceptAll},
		{Name: "b", URL: "http://x/b", Validate: acceptAll},
	}
	g := &fakeGetter{bodies: map[string]string{"http://x/a": "conteúdo a", "http://x/b": "conteúdo b"}}
	s := &fakeFileSaver{unchanged: map[string]bool{"b": true}}

	stats, err := Files(context.Background(), discard, g, s, 1, "teste", files)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (FilesStats{Saved: 1, Unchanged: 1}) {
		t.Errorf("stats = %+v", stats)
	}
	if len(s.saved) != 1 {
		t.Fatalf("gravados = %+v", s.saved)
	}
	got := s.saved[0]
	if got.source != "teste" || got.name != "a" || got.content != "conteúdo a" ||
		got.size != int64(len("conteúdo a")) || len(got.sha) != 64 {
		t.Errorf("gravado = %+v", got)
	}
}

func TestFiles_OneFailureDoesNotStopTheOthers(t *testing.T) {
	files := []RemoteFile{
		{Name: "fora-do-ar", URL: "http://x/a", Validate: acceptAll},
		{Name: "html", URL: "http://x/b", Validate: validateXLSX},
		{Name: "ok", URL: "http://x/c", Validate: validateXLSX},
	}
	g := &fakeGetter{
		bodies: map[string]string{"http://x/b": "<html>erro</html>", "http://x/c": "PK\x03\x04resto"},
		errs:   map[string]error{"http://x/a": errors.New("timeout")},
	}
	s := &fakeFileSaver{}

	stats, err := Files(context.Background(), discard, g, s, 1, "teste", files)
	if err == nil {
		t.Fatal("quer erro")
	}
	for _, want := range []string{"fora-do-ar", "timeout", "html", "xlsx"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("erro %q não menciona %q", err, want)
		}
	}
	if stats != (FilesStats{Saved: 1, Failed: 2}) {
		t.Errorf("stats = %+v", stats)
	}
	if len(s.saved) != 1 || s.saved[0].name != "ok" {
		t.Errorf("gravados = %+v", s.saved)
	}
}
