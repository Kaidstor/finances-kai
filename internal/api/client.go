// Package api — клиент к HTTP-API приложения. Все ответы приходят конвертом
// {success, data, error}; клиент его разворачивает и отдаёт наружу data либо
// ошибку с текстом сервера.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// ErrUnauthorized выделен отдельно: на него команды советуют перелогиниться,
// а не показывают сырое «401».
var ErrUnauthorized = errors.New("не авторизован")

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("%s %s: читаю ответ: %w", method, path, err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		// Не JSON — почти всегда прокси или страница ошибки перед приложением.
		return fmt.Errorf("%s %s: сервер ответил %s, а не JSON: %s",
			method, path, resp.Status, snippet(raw))
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s", ErrUnauthorized, env.Error)
	}
	if !env.Success {
		if env.Error == "" {
			env.Error = resp.Status
		}
		return fmt.Errorf("%s %s: %s", method, path, env.Error)
	}

	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("%s %s: разбираю data: %w", method, path, err)
		}
	}
	return nil
}

func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		s = "(пустой ответ)"
	}
	return s
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	return u, c.do(ctx, http.MethodGet, "/api/auth/me", nil, &u)
}

func (c *Client) Payments(ctx context.Context) ([]Payment, error) {
	var out []Payment
	return out, c.do(ctx, http.MethodGet, "/api/payments", nil, &out)
}

func (c *Client) Payment(ctx context.Context, id string) (Payment, error) {
	var out Payment
	return out, c.do(ctx, http.MethodGet, "/api/payments/"+url.PathEscape(id), nil, &out)
}

func (c *Client) CreatePayment(ctx context.Context, p CreatePayment) (Payment, error) {
	var out Payment
	return out, c.do(ctx, http.MethodPost, "/api/payments", p, &out)
}

func (c *Client) DeletePayment(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/payments/"+url.PathEscape(id), nil, nil)
}

func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	var out []Tag
	return out, c.do(ctx, http.MethodGet, "/api/tags", nil, &out)
}

func (c *Client) CreateTag(ctx context.Context, t CreateTag) (Tag, error) {
	var out Tag
	return out, c.do(ctx, http.MethodPost, "/api/tags", t, &out)
}

func (c *Client) Counterparties(ctx context.Context) ([]Counterparty, error) {
	var out []Counterparty
	return out, c.do(ctx, http.MethodGet, "/api/counterparties", nil, &out)
}

func (c *Client) CreateCounterparty(ctx context.Context, cp CreateCounterparty) (Counterparty, error) {
	var out Counterparty
	return out, c.do(ctx, http.MethodPost, "/api/counterparties", cp, &out)
}

func (c *Client) Templates(ctx context.Context) ([]Template, error) {
	var out []Template
	return out, c.do(ctx, http.MethodGet, "/api/templates", nil, &out)
}

// Export отдаёт zip со счетами. Возвращает содержимое и имя файла, которое
// предложил сервер (из Content-Disposition).
func (c *Client) Export(ctx context.Context, query url.Values) ([]byte, string, error) {
	path := "/api/payments/export"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("экспорт: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("экспорт: читаю ответ: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var env envelope
		if json.Unmarshal(raw, &env) == nil && env.Error != "" {
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, "", fmt.Errorf("%w: %s", ErrUnauthorized, env.Error)
			}
			return nil, "", fmt.Errorf("экспорт: %s", env.Error)
		}
		return nil, "", fmt.Errorf("экспорт: сервер ответил %s", resp.Status)
	}

	name := "invoices.zip"
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		if v := params["filename"]; v != "" {
			name = v
		}
	}
	return raw, name, nil
}
