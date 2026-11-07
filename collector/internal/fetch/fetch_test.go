package fetch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type reply struct {
	status int
	body   string
}

// newTestClient sobe um servidor local que devolve as respostas em ordem
// (repetindo a última) e conta as requisições e o User-Agent recebidos.
func newTestClient(t *testing.T, replies ...reply) (*Client, string, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var calls atomic.Int32
	var userAgent atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		userAgent.Store(r.Header.Get("User-Agent"))
		rep := replies[min(n, len(replies)-1)]
		w.WriteHeader(rep.status)
		_, _ = io.WriteString(w, rep.body)
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		HTTPClient:   srv.Client(),
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxRetries:   2,
		BaseBackoff:  time.Millisecond,
		MaxBodyBytes: DefaultMaxBodyBytes,
	}
	return c, srv.URL, &calls, &userAgent
}

func TestGet(t *testing.T) {
	tests := []struct {
		name       string
		replies    []reply
		wantBody   string
		wantStatus int // status do *StatusError esperado; 0 = sem erro de status
		wantErr    bool
		wantCalls  int32
	}{
		{"200 devolve o corpo", []reply{{200, "a;b;c"}}, "a;b;c", 0, false, 1},
		{"404 não repete", []reply{{404, "não achei"}}, "", 404, true, 1},
		{"403 não repete", []reply{{403, ""}}, "", 403, true, 1},
		{"500 seguido de 200 repete", []reply{{500, ""}, {200, "ok"}}, "ok", 0, false, 2},
		{"429 seguido de 200 repete", []reply{{429, ""}, {200, "ok"}}, "ok", 0, false, 2},
		{"503 sempre desiste após MaxRetries+1", []reply{{503, ""}}, "", 503, true, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, url, calls, _ := newTestClient(t, tt.replies...)

			body, err := c.Get(context.Background(), url)

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, quer %q", body, tt.wantBody)
			}
			if tt.wantStatus != 0 {
				var se *StatusError
				if !errors.As(err, &se) || se.StatusCode != tt.wantStatus {
					t.Errorf("err = %v, quer StatusError %d", err, tt.wantStatus)
				}
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("chamadas = %d, quer %d", got, tt.wantCalls)
			}
		})
	}
}

func TestGet_SendsUserAgent(t *testing.T) {
	c, url, _, ua := newTestClient(t, reply{200, "ok"})
	if _, err := c.Get(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	if got := ua.Load(); got != UserAgent {
		t.Errorf("User-Agent = %q, quer %q", got, UserAgent)
	}
}

func TestGet_RejectsBodyAboveLimit(t *testing.T) {
	c, url, _, _ := newTestClient(t, reply{200, strings.Repeat("x", 11)})
	c.MaxBodyBytes = 10

	_, err := c.Get(context.Background(), url)
	if err == nil || !strings.Contains(err.Error(), "limite") {
		t.Fatalf("esperava erro de limite, veio %v", err)
	}
}

func TestGet_ContextCanceledStopsRetrying(t *testing.T) {
	c, url, calls, _ := newTestClient(t, reply{503, ""})
	c.BaseBackoff = time.Hour // sem cancelamento, o teste travaria aqui

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := c.Get(ctx, url); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, quer context.DeadlineExceeded", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("chamadas = %d, quer 1", got)
	}
}

func TestBackoff_DoublesWithJitter(t *testing.T) {
	for attempt, base := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second} {
		got := Backoff(time.Second, attempt)
		if got < base || got > base+base/5 {
			t.Errorf("Backoff(1s, %d) = %v, quer entre %v e %v", attempt, got, base, base+base/5)
		}
	}
}
