package sgs

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// reply é uma resposta fixa que o servidor de teste devolve.
type reply struct {
	status int
	body   string
}

// lastURL guarda a URL da última requisição recebida pelo servidor de teste.
// O handler roda em outra goroutine, então o acesso é protegido por mutex.
type lastURL struct {
	mu  sync.Mutex
	url *url.URL
}

func (l *lastURL) get() *url.URL {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.url
}

// newTestClient sobe um servidor HTTP local que devolve as respostas em
// ordem (repetindo a última) e um Client apontado para ele, sem espera
// entre tentativas. Devolve também um contador de requisições recebidas.
func newTestClient(t *testing.T, replies ...reply) (*Client, *atomic.Int32, *lastURL) {
	t.Helper()
	var calls atomic.Int32
	last := &lastURL{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		last.mu.Lock()
		last.url = r.URL
		last.mu.Unlock()
		rep := replies[min(n, len(replies)-1)]
		w.WriteHeader(rep.status)
		_, _ = io.WriteString(w, rep.body)
	}))
	// t.Cleanup roda ao fim do teste, como um defer do próprio teste.
	t.Cleanup(srv.Close)

	c := &Client{
		BaseURL:     srv.URL,
		HTTPClient:  srv.Client(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxRetries:  2,
		BaseBackoff: time.Millisecond,
	}
	return c, &calls, last
}

const okBody = `[{"data":"06/11/2026","valor":"5.5013"}]`

func TestFetch_Success(t *testing.T) {
	c, calls, last := newTestClient(t, reply{200, okBody})

	req := Request{
		SeriesCode: 1,
		Start:      time.Date(2016, 11, 7, 0, 0, 0, 0, time.UTC),
		End:        time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC),
	}
	resp, err := c.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if string(resp.Body) != okBody {
		t.Errorf("body = %s, quer %s", resp.Body, okBody)
	}
	if resp.StatusCode != 200 || resp.Empty() {
		t.Errorf("status = %d, Empty = %v", resp.StatusCode, resp.Empty())
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("chamadas = %d, quer 1", got)
	}
	u := last.get()
	if u.Path != "/dados/serie/bcdata.sgs.1/dados" {
		t.Errorf("path = %s", u.Path)
	}
	q := u.Query()
	if q.Get("formato") != "json" || q.Get("dataInicial") != "07/11/2016" || q.Get("dataFinal") != "06/11/2026" {
		t.Errorf("query = %s", u.RawQuery)
	}
}

func TestFetch_NoDatesOmitsParams(t *testing.T) {
	c, _, last := newTestClient(t, reply{200, okBody})

	if _, err := c.Fetch(context.Background(), Request{SeriesCode: 433}); err != nil {
		t.Fatal(err)
	}
	if u := last.get(); u.Query().Has("dataInicial") || u.Query().Has("dataFinal") {
		t.Errorf("não deveria enviar datas: %s", u.RawQuery)
	}
}

// Testes de tabela (table-driven) são o padrão em Go: cada caso é uma linha
// de dados, e t.Run cria um subteste com nome para cada uma.
func TestFetch_StatusHandling(t *testing.T) {
	tests := []struct {
		name      string
		replies   []reply
		wantErr   bool
		wantEmpty bool
		wantCalls int32
	}{
		{
			name:      "404 sem valores no período é resposta vazia",
			replies:   []reply{{404, `{"erro":{"statusCode":404,"detail":"SGSNegocioException: Value(s) not found"}}`}},
			wantEmpty: true,
			wantCalls: 1,
		},
		{
			name:      "404 de série inexistente é erro e não repete",
			replies:   []reply{{404, `A serie nao existe!`}},
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name:      "406 (janela maior que 10 anos) não repete",
			replies:   []reply{{406, `{"error":"janela de no máximo 10 anos"}`}},
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name:      "500 seguido de 200 repete e funciona",
			replies:   []reply{{500, `erro`}, {200, okBody}},
			wantCalls: 2,
		},
		{
			name:      "429 seguido de 200 repete e funciona",
			replies:   []reply{{429, ``}, {200, okBody}},
			wantCalls: 2,
		},
		{
			// Visto em nov/2026: o SGS respondeu 304 a uma consulta sem
			// cabeçalhos de cache, e a coleta inteira falhou.
			name:      "304 sem ter pedido cache repete e funciona",
			replies:   []reply{{304, ``}, {200, okBody}},
			wantCalls: 2,
		},
		{
			name:      "200 com HTML repete",
			replies:   []reply{{200, `<html>manutenção</html>`}, {200, okBody}},
			wantCalls: 2,
		},
		{
			name:      "503 sempre desiste após MaxRetries+1 tentativas",
			replies:   []reply{{503, `indisponível`}},
			wantErr:   true,
			wantCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, calls, _ := newTestClient(t, tt.replies...)

			resp, err := c.Fetch(context.Background(), Request{SeriesCode: 1})

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && resp.Empty() != tt.wantEmpty {
				t.Errorf("Empty = %v, quer %v", resp.Empty(), tt.wantEmpty)
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("chamadas = %d, quer %d", got, tt.wantCalls)
			}
		})
	}
}

func TestFetch_ContextCanceledStopsRetrying(t *testing.T) {
	c, calls, _ := newTestClient(t, reply{503, ``})
	c.BaseBackoff = time.Hour // sem cancelamento, o teste travaria aqui

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.Fetch(ctx, Request{SeriesCode: 1})
	if err == nil {
		t.Fatal("esperava erro de contexto")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("chamadas = %d, quer 1", got)
	}
}

func TestIsStatus(t *testing.T) {
	c, _, _ := newTestClient(t, reply{406, `{}`})
	_, err := c.Fetch(context.Background(), Request{SeriesCode: 1})
	if !IsStatus(err, 406) {
		t.Errorf("IsStatus(err, 406) = false; err = %v", err)
	}
}
