// Package sgs é um cliente da API SGS (Sistema Gerenciador de Séries
// Temporais) do Banco Central. Ele devolve o corpo das respostas sem
// interpretar os dados: a interpretação é papel do pipeline.
//
// Formato da API (conferido em 2026-11-07):
//
//	GET {base}/dados/serie/bcdata.sgs.{código}/dados?formato=json&dataInicial=dd/MM/aaaa&dataFinal=dd/MM/aaaa
//	200 -> [{"data":"06/11/2026","valor":"5.5013"}, ...]
//	404 -> {"erro":{"detail":"... Value(s) not found"}}  (período sem dados)
//	404 -> "A serie nao existe!"                          (código inválido)
//	406 -> janela maior que 10 anos em série diária
package sgs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/LuizAPLeite/radar-economico/collector/internal/fetch"
)

// DefaultBaseURL é o endereço público da API SGS.
const DefaultBaseURL = "https://api.bcb.gov.br"

// dateLayout é o formato de data que a API espera: dd/MM/aaaa.
// Em Go, layouts de data são escritos com a data de referência
// "Mon Jan 2 15:04:05 2006", e não com letras como dd/MM/yyyy.
const dateLayout = "02/01/2006"

// Request descreve uma consulta a uma série. Start e End zerados
// significam "sem filtro de datas" (só aceito em séries não diárias).
type Request struct {
	SeriesCode int
	Start      time.Time
	End        time.Time
}

// Response é o resultado bruto de uma consulta.
type Response struct {
	Request    Request
	URL        string
	StatusCode int
	// Body é o JSON devolvido pela API, sem alteração.
	Body json.RawMessage
}

// Empty informa se a API respondeu que não há dados no período.
func (r Response) Empty() bool { return r.StatusCode == http.StatusNotFound }

// StatusError é devolvido quando a API responde com um status inesperado.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sgs: status %d: %s", e.StatusCode, e.Body)
}

// Client faz consultas à API SGS com timeout e retry.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Logger     *slog.Logger
	// MaxRetries é quantas vezes uma falha temporária é repetida
	// (o total de tentativas é MaxRetries + 1).
	MaxRetries int
	// BaseBackoff é a espera antes da primeira repetição; ela dobra a cada tentativa.
	BaseBackoff time.Duration
}

// NewClient cria um Client com valores padrão razoáveis.
func NewClient(logger *slog.Logger) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		// Consultas de 10 anos levam ~5 s; 60 s dá folga sem travar para sempre.
		HTTPClient:  &http.Client{Timeout: 60 * time.Second},
		Logger:      logger,
		MaxRetries:  3,
		BaseBackoff: time.Second,
	}
}

// Fetch consulta uma série. Falhas temporárias (rede, timeout, 429, 5xx,
// corpo que não é JSON) são repetidas com backoff exponencial; erros do
// cliente (4xx) não, porque repetir não muda o resultado.
func (c *Client) Fetch(ctx context.Context, req Request) (Response, error) {
	u := c.buildURL(req)

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := fetch.Backoff(c.BaseBackoff, attempt)
			c.Logger.Warn("repetindo consulta ao SGS",
				"series", req.SeriesCode, "attempt", attempt, "wait", wait, "err", lastErr)
			if err := fetch.Sleep(ctx, wait); err != nil {
				return Response{}, err
			}
		}

		resp, retry, err := c.do(ctx, req, u)
		if err == nil {
			return resp, nil
		}
		if !retry {
			return Response{}, err
		}
		lastErr = err
	}
	return Response{}, fmt.Errorf("sgs: série %d: desistindo após %d tentativas: %w",
		req.SeriesCode, c.MaxRetries+1, lastErr)
}

// do faz uma única tentativa. Devolve se vale a pena tentar de novo.
func (c *Client) do(ctx context.Context, req Request, u string) (resp Response, retry bool, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Response{}, false, fmt.Errorf("sgs: montando requisição: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		// Se o contexto foi cancelado, não adianta repetir.
		if ctx.Err() != nil {
			return Response{}, false, ctx.Err()
		}
		return Response{}, true, fmt.Errorf("sgs: requisição: %w", err)
	}
	// defer roda quando a função retorna: garante que o corpo é fechado
	// em todos os caminhos, liberando a conexão para reuso.
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, true, fmt.Errorf("sgs: lendo corpo: %w", err)
	}

	result := Response{Request: req, URL: u, StatusCode: httpResp.StatusCode, Body: body}

	switch {
	case httpResp.StatusCode == http.StatusOK:
		if !json.Valid(body) {
			// O BCB às vezes devolve uma página HTML de manutenção com status 200.
			return Response{}, true, fmt.Errorf("sgs: resposta 200 não é JSON: %s", snippet(body))
		}
		return result, false, nil

	case httpResp.StatusCode == http.StatusNotFound && isNoValues(body):
		return result, false, nil

	case httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= 500:
		return Response{}, true, &StatusError{StatusCode: httpResp.StatusCode, Body: snippet(body)}

	default:
		return Response{}, false, &StatusError{StatusCode: httpResp.StatusCode, Body: snippet(body)}
	}
}

func (c *Client) buildURL(req Request) string {
	q := url.Values{}
	q.Set("formato", "json")
	if !req.Start.IsZero() {
		q.Set("dataInicial", req.Start.Format(dateLayout))
	}
	if !req.End.IsZero() {
		q.Set("dataFinal", req.End.Format(dateLayout))
	}
	return c.BaseURL + "/dados/serie/bcdata.sgs." + strconv.Itoa(req.SeriesCode) + "/dados?" + q.Encode()
}

// isNoValues reconhece o 404 que significa "período sem dados",
// diferente do 404 de série inexistente.
func isNoValues(body []byte) bool {
	return json.Valid(body) && bytes.Contains(body, []byte("Value(s) not found"))
}

func snippet(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}

// IsStatus informa se err é um StatusError com o código dado.
func IsStatus(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.StatusCode == code
}
