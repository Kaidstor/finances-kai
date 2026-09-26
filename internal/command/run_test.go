package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/Kaidstor/finances-kai/internal/api"
	"github.com/Kaidstor/finances-kai/internal/config"
	"github.com/Kaidstor/finances-kai/internal/exit"
	"github.com/Kaidstor/finances-kai/internal/output"
)

// Коды выхода и форма конверта — контракт с агентом и скриптами. Тесты
// гоняют Run целиком против фейкового приложения.

type envelope struct {
	V       int             `json:"v"`
	Command string          `json:"command"`
	Exit    int             `json:"exit"`
	Data    json.RawMessage `json:"data"`
	Warning []string        `json:"warning"`
	Error   *output.Failure `json:"error"`
}

func fakeApp(t *testing.T, routes map[string]func(w http.ResponseWriter)) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("неожиданный запрос %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		h(w)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("FINANCES_KAI_HOME", t.TempDir())
	t.Setenv("FINANCES_KAI_URL", srv.URL)
	t.Setenv("FINANCES_KAI_TOKEN", "test-token")
	t.Setenv("FINANCES_KAI_PROFILE", "")
	t.Setenv("FINANCES_KAI_TOKEN_REF", "")
}

func reply(status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

// run запускает Run, перехватив stdout и stderr.
func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	inR, inW, _ := os.Pipe()
	inW.Close()
	origIn, origOut, origErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = inR, outW, errW

	var outBuf, errBuf bytes.Buffer
	done := make(chan struct{}, 2)
	go func() { io.Copy(&outBuf, outR); done <- struct{}{} }()
	go func() { io.Copy(&errBuf, errR); done <- struct{}{} }()

	code = Run(args)

	outW.Close()
	errW.Close()
	<-done
	<-done
	os.Stdin, os.Stdout, os.Stderr = origIn, origOut, origErr
	inR.Close()
	return code, outBuf.String(), errBuf.String()
}

func decode(t *testing.T, stdout string) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("stdout не конверт: %v\n%s", err, stdout)
	}
	return e
}

const paymentJSON = `{"id":"11111111-2222-3333-4444-555555555555","status":"paid","amount":"-250.00",
"currency":"RUB","amountInBaseCurrency":"-250.00","baseCurrency":"RUB","date":"2026-09-01 00:00:00",
"description":"кофе","tags":[],"counterparties":[]}`

func TestJSONSuccessEnvelope(t *testing.T) {
	fakeApp(t, map[string]func(http.ResponseWriter){
		"GET /api/payments": reply(200, `{"success":true,"data":[`+paymentJSON+`]}`),
	})

	code, stdout, _ := run(t, "ls", "--period", "all", "--json")
	if code != exit.OK {
		t.Fatalf("код %d, want 0", code)
	}
	e := decode(t, stdout)
	if e.V != 1 || e.Command != "ls" || e.Exit != 0 || e.Error != nil {
		t.Errorf("конверт: %+v", e)
	}
	var rows []api.Payment
	if err := json.Unmarshal(e.Data, &rows); err != nil || len(rows) != 1 || rows[0].Description == nil {
		t.Errorf("data: %s (%v)", e.Data, err)
	}
}

func TestJSONFlagAnywhere(t *testing.T) {
	fakeApp(t, map[string]func(http.ResponseWriter){
		"GET /api/payments": reply(200, `{"success":true,"data":[]}`),
	})
	code, stdout, _ := run(t, "--json", "ls")
	if code != exit.OK || decode(t, stdout).Command != "ls" {
		t.Fatalf("--json перед командой не сработал: код %d, %s", code, stdout)
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		routes map[string]func(http.ResponseWriter)
		args   []string
		code   int
		kind   string
		// jsonOnly — отказ только в режиме --json, текстом команда законна.
		jsonOnly bool
	}{
		{
			name:   "платёж по UUID: 404 от приложения",
			routes: map[string]func(http.ResponseWriter){"GET /api/payments/11111111-2222-3333-4444-555555555555": reply(404, `{"success":false,"error":"Платёж не найден"}`)},
			args:   []string{"show", "11111111-2222-3333-4444-555555555555"},
			code:   exit.NotFound, kind: "not_found",
		},
		{
			name:   "платёж по префиксу не найден локально",
			routes: map[string]func(http.ResponseWriter){"GET /api/payments": reply(200, `{"success":true,"data":[`+paymentJSON+`]}`)},
			args:   []string{"show", "99999999"},
			code:   exit.NotFound, kind: "not_found",
		},
		{
			name:   "тег не найден",
			routes: map[string]func(http.ResponseWriter){"GET /api/payments": reply(200, `{"success":true,"data":[]}`), "GET /api/tags": reply(200, `{"success":true,"data":[]}`)},
			args:   []string{"ls", "--tag", "нет-такого"},
			code:   exit.NotFound, kind: "not_found",
		},
		{
			name:   "токен отозван",
			routes: map[string]func(http.ResponseWriter){"GET /api/payments": reply(401, `{"success":false,"error":"Unauthorized"}`)},
			args:   []string{"ls"},
			code:   exit.Tool, kind: "auth",
		},
		{
			name:   "приложение отвергло платёж",
			routes: map[string]func(http.ResponseWriter){"POST /api/payments": reply(400, `{"success":false,"error":"Validation error"}`)},
			args:   []string{"add", "-250", "кофе", "--currency", "RUB"},
			code:   exit.NotApplied, kind: "api",
		},
		{
			name:   "прокси вместо приложения",
			routes: map[string]func(http.ResponseWriter){"GET /api/payments": func(w http.ResponseWriter) { w.WriteHeader(502); io.WriteString(w, "<html>Bad Gateway</html>") }},
			args:   []string{"ls"},
			code:   exit.Tool, kind: "network",
		},
		{
			name:   "удаление без --yes и без TTY",
			routes: map[string]func(http.ResponseWriter){"GET /api/payments": reply(200, `{"success":true,"data":[`+paymentJSON+`]}`)},
			args:   []string{"rm", "11111111"},
			code:   exit.Tool, kind: "confirm",
		},
		{name: "неизвестная команда", args: []string{"bogus"}, code: exit.Tool, kind: "usage"},
		{name: "неверный флаг", args: []string{"ls", "--bogus"}, code: exit.Tool, kind: "usage"},
		{name: "неверный период", args: []string{"ls", "--period", "decade"}, code: exit.Tool, kind: "usage"},
		{name: "export в stdout с --json", args: []string{"export", "-o", "-"}, code: exit.Tool, kind: "usage", jsonOnly: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeApp(t, c.routes)

			code, stdout, _ := run(t, append(c.args, "--json")...)
			if code != c.code {
				t.Errorf("--json: код %d, want %d", code, c.code)
			}
			e := decode(t, stdout)
			if e.Exit != c.code || e.Error == nil || e.Error.Kind != c.kind || e.Error.Message == "" {
				t.Errorf("конверт отказа: %+v, error=%+v", e, e.Error)
			}

			if c.jsonOnly {
				return
			}
			code, stdout, stderr := run(t, c.args...)
			if code != c.code {
				t.Errorf("текст: код %d, want %d", code, c.code)
			}
			if stderr == "" {
				t.Error("текст: отказ не напечатан в stderr")
			}
			if c.kind != "confirm" && stdout != "" {
				t.Errorf("текст: при отказе что-то ушло в stdout: %q", stdout)
			}
		})
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"ls", "-h"}, {"add", "-h"}, {"--help"}} {
		if code, _, _ := run(t, args...); code != exit.OK {
			t.Errorf("%v: код %d, want 0", args, code)
		}
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		kind string
	}{
		{"таймаут http-клиента", fmt.Errorf("GET /api/payments: %w",
			&url.Error{Op: "Get", URL: "x", Err: timeoutErr{}}), exit.Timeout, "timeout"},
		{"нет связи", &url.Error{Op: "Get", URL: "x", Err: errors.New("connection refused")}, exit.Tool, "network"},
		{"504 от прокси", &api.Error{Status: 504, NotJSON: true}, exit.Timeout, "timeout"},
		{"500 с JSON", &api.Error{Status: 500, Message: "boom"}, exit.NotApplied, "api"},
		{"403", &api.Error{Status: 403}, exit.Tool, "auth"},
		{"профиль не найден", &config.ProfileNotFoundError{Name: "x"}, exit.NotFound, "not_found"},
		{"профиль не выбран", config.ErrNoProfile, exit.Tool, "config"},
		{"неоднозначный тег", ambiguous("тег подходит к нескольким"), exit.Tool, "ambiguous"},
	}
	for _, c := range cases {
		code, kind := classify(c.err)
		if code != c.code || kind != c.kind {
			t.Errorf("%s: (%d, %s), want (%d, %s)", c.name, code, kind, c.code, c.kind)
		}
	}
}

func TestSplitGlobalFlags(t *testing.T) {
	asJSON, rest := splitGlobalFlags([]string{"show", "1111", "--json", "--profile", "x"})
	if !asJSON || !reflect.DeepEqual(rest, []string{"show", "1111", "--profile", "x"}) {
		t.Errorf("got (%v, %v)", asJSON, rest)
	}
	asJSON, _ = splitGlobalFlags([]string{"--json", "ls", "--human"})
	if asJSON {
		t.Error("последний из --json/--human должен побеждать")
	}
	asJSON, rest = splitGlobalFlags([]string{"add", "-1", "--", "--json"})
	if asJSON || !reflect.DeepEqual(rest, []string{"add", "-1", "--", "--json"}) {
		t.Errorf("после -- флаги не вынимаются: (%v, %v)", asJSON, rest)
	}
}
