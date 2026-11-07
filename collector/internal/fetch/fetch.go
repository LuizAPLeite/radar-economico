// Package fetch baixa conteúdo por HTTP GET, com timeout e retry. É usado
// pelas fontes que só precisam do corpo da resposta (IBGE, ANP).
package fetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"
)

// UserAgent identifica o coletor para quem opera os servidores das fontes.
const UserAgent = "radar-economico/1.0 (+https://github.com/LuizAPLeite/radar-economico)"

// DefaultMaxBodyBytes limita o tamanho de uma resposta. Os maiores arquivos
// da ANP têm ~20 MB; o limite protege a memória se uma URL devolver algo
// inesperado.
const DefaultMaxBodyBytes = 100 << 20 // 100 MB

// StatusError é devolvido quando o servidor responde com status diferente de 200.
type StatusError struct {
	URL        string
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("fetch: %s respondeu %d", e.URL, e.StatusCode)
}

// Client baixa URLs repetindo as falhas temporárias.
type Client struct {
	HTTPClient   *http.Client
	Logger       *slog.Logger
	MaxRetries   int           // repetições após a primeira tentativa
	BaseBackoff  time.Duration // espera antes da 1ª repetição; dobra a cada uma
	MaxBodyBytes int64
}

// NewClient cria um Client com o timeout dado para cada requisição inteira
// (incluindo baixar o corpo).
func NewClient(logger *slog.Logger, timeout time.Duration) *Client {
	return &Client{
		HTTPClient:   &http.Client{Timeout: timeout},
		Logger:       logger,
		MaxRetries:   3,
		BaseBackoff:  2 * time.Second,
		MaxBodyBytes: DefaultMaxBodyBytes,
	}
}

// Get baixa url e devolve o corpo. Repete em erro de rede, timeout, 429 e
// 5xx; qualquer outro status diferente de 200 vira um *StatusError na hora.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := Backoff(c.BaseBackoff, attempt)
			c.Logger.Warn("repetindo download", "url", url, "attempt", attempt, "wait", wait.String(), "err", lastErr)
			if err := Sleep(ctx, wait); err != nil {
				return nil, err
			}
		}

		body, retry, err := c.get(ctx, url)
		if err == nil {
			return body, nil
		}
		if !retry {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("fetch: %s: desistindo após %d tentativas: %w", url, c.MaxRetries+1, lastErr)
}

// get faz uma tentativa e diz se vale a pena repetir em caso de erro.
func (c *Client) get(ctx context.Context, url string) (body []byte, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("fetch: montando requisição: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, true, fmt.Errorf("fetch: %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Ler (pouco) do corpo antes de fechar deixa a conexão ser reaproveitada.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retry, &StatusError{URL: url, StatusCode: resp.StatusCode}
	}

	// LimitReader para em MaxBodyBytes+1: se ler isso tudo, a resposta passou do limite.
	body, err = io.ReadAll(io.LimitReader(resp.Body, c.MaxBodyBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("fetch: lendo %s: %w", url, err)
	}
	if int64(len(body)) > c.MaxBodyBytes {
		return nil, false, fmt.Errorf("fetch: %s passa do limite de %d bytes", url, c.MaxBodyBytes)
	}
	return body, false, nil
}

// Backoff devolve base * 2^(attempt-1) mais até 20% de variação aleatória
// (jitter), para que downloads em paralelo não repitam todos no mesmo instante.
func Backoff(base time.Duration, attempt int) time.Duration {
	d := base << (attempt - 1)
	return d + time.Duration(rand.Int64N(int64(d)/5+1))
}

// Sleep espera d, mas retorna antes, com erro, se o contexto for cancelado.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
