package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Testes em Go: funções TestXxx(t *testing.T) em arquivos *_test.go.
// O httptest permite exercitar handlers sem abrir uma porta de verdade.
func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
	}{
		{"GET retorna 200", http.MethodGet, http.StatusOK},
		{"POST não é permitido", http.MethodPost, http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			rec := httptest.NewRecorder()

			newRouter().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, quer %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
