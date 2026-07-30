package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBearerAndEnvelope(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]string{"id": "u1", "email": "a@b.c", "defaultCurrency": "RUB"},
		})
	}))
	defer srv.Close()

	user, err := New(srv.URL, "tok123").Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if user.Email != "a@b.c" || user.DefaultCurrency != "RUB" {
		t.Errorf("распарсили не то: %+v", user)
	}
}

func TestUnauthorizedIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Unauthorized"})
	}))
	defer srv.Close()

	_, err := New(srv.URL, "bad").Me(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("ожидался ErrUnauthorized, получено %v", err)
	}
}

// Ошибка внутри конверта приходит с кодом 200 — без разбора success клиент
// принял бы её за успех и вернул пустые данные.
func TestEnvelopeErrorOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Validation error"})
	}))
	defer srv.Close()

	_, err := New(srv.URL, "t").Payments(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Validation error") {
		t.Fatalf("ожидалась ошибка валидации, получено %v", err)
	}
}

// Перед приложением стоит traefik: его страница ошибки — HTML, и клиент должен
// сказать это прямо, а не «unexpected character».
func TestNonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "t").Payments(context.Background())
	if err == nil || !strings.Contains(err.Error(), "а не JSON") {
		t.Fatalf("ожидалось сообщение про не-JSON, получено %v", err)
	}
}

func TestCreatePaymentBody(t *testing.T) {
	var body CreatePayment
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/payments" {
			t.Errorf("неожиданный запрос %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]string{"id": "p1", "amount": "-250", "currency": "RUB"},
		})
	}))
	defer srv.Close()

	desc := "кофе"
	_, err := New(srv.URL, "t").CreatePayment(context.Background(), CreatePayment{
		Status: "paid", Amount: "-250", Currency: "RUB", Date: "2026-07-15",
		Description: &desc, TagIDs: []string{"t1"},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if body.Amount != "-250" || body.Date != "2026-07-15" || body.Status != "paid" {
		t.Errorf("сервер получил не то: %+v", body)
	}
	if body.Description == nil || *body.Description != "кофе" {
		t.Errorf("описание не доехало: %+v", body.Description)
	}
	if len(body.TagIDs) != 1 || body.TagIDs[0] != "t1" {
		t.Errorf("теги не доехали: %+v", body.TagIDs)
	}
}

func TestExportFilename(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("from"); got != "2026-01-01" {
			t.Errorf("from = %q", got)
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="invoices_01.01.2026-31.03.2026.zip"`)
		w.Write([]byte("PK\x03\x04zip"))
	}))
	defer srv.Close()

	data, name, err := New(srv.URL, "t").Export(context.Background(),
		map[string][]string{"from": {"2026-01-01"}})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if name != "invoices_01.01.2026-31.03.2026.zip" {
		t.Errorf("имя файла = %q", name)
	}
	if !strings.HasPrefix(string(data), "PK") {
		t.Errorf("содержимое не похоже на zip: %q", data)
	}
}

// Экспорт возвращает 404 «платежей не найдено» конвертом, а не zip-ом.
func TestExportErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false, "error": "Платежей за указанный период не найдено",
		})
	}))
	defer srv.Close()

	_, _, err := New(srv.URL, "t").Export(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "не найдено") {
		t.Fatalf("ожидалась ошибка сервера, получено %v", err)
	}
}
