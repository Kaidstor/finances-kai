package command

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/Kaidstor/finances-kai/internal/api"
	"github.com/Kaidstor/finances-kai/internal/config"
	"github.com/Kaidstor/finances-kai/internal/exit"
	"github.com/Kaidstor/finances-kai/internal/keyring"
	"github.com/Kaidstor/finances-kai/internal/output"
)

// cliError — отказ с уже известным классом и кодом выхода.
type cliError struct {
	code int
	kind string
	err  error
	// printed — диагностику уже напечатал пакет flag или usage команды; в
	// текстовом режиме второй раз её не печатаем.
	printed bool
}

func (e *cliError) Error() string { return e.err.Error() }
func (e *cliError) Unwrap() error { return e.err }

func classed(code int, kind, format string, a ...any) error {
	return &cliError{code: code, kind: kind, err: fmt.Errorf(format, a...)}
}

func usageErr(format string, a ...any) error {
	return classed(exit.Tool, "usage", format, a...)
}

func notFound(format string, a ...any) error {
	return classed(exit.NotFound, "not_found", format, a...)
}

func ambiguous(format string, a ...any) error {
	return classed(exit.Tool, "ambiguous", format, a...)
}

func configErr(err error) error {
	if err == nil {
		return nil
	}
	return &cliError{code: exit.Tool, kind: "config", err: err}
}

func authErr(format string, a ...any) error {
	return classed(exit.Tool, "auth", format, a...)
}

// errHelp — запрошена справка (-h); она уже напечатана, выходим с нулём.
var errHelp = errors.New("справка")

// parseFlags — fs.Parse с классом ошибки: неверный флаг — usage и код 2.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return errHelp
		}
		return &cliError{code: exit.Tool, kind: "usage", err: err, printed: true}
	}
	return nil
}

// errUsage показывает справку команды и отказывает с кодом 2. Текст msg уходит
// только в конверт: человек уже видит справку.
func errUsage(fs *flag.FlagSet, format string, a ...any) error {
	fs.Usage()
	return &cliError{code: exit.Tool, kind: "usage", err: fmt.Errorf(format, a...), printed: true}
}

// classify переводит ошибку в код выхода и kind конверта.
func classify(err error) (code int, kind string) {
	var ce *cliError
	var apiErr *api.Error
	var noProfile *config.ProfileNotFoundError
	var netErr net.Error

	switch {
	case errors.Is(err, context.Canceled):
		return exit.Interrupted, "interrupted"
	case errors.Is(err, context.DeadlineExceeded):
		return exit.Timeout, "timeout"
	case errors.As(err, &ce):
		return ce.code, ce.kind
	case errors.Is(err, api.ErrUnauthorized):
		return exit.Tool, "auth"
	case errors.As(err, &noProfile):
		return exit.NotFound, "not_found"
	case errors.Is(err, config.ErrNoProfile):
		return exit.Tool, "config"
	case errors.Is(err, keyring.ErrNotFound):
		return exit.Tool, "auth"
	case errors.As(err, &apiErr):
		return classifyAPI(apiErr)
	case errors.As(err, &netErr):
		// http.Client с Timeout отдаёт url.Error, у которого Timeout() == true.
		if netErr.Timeout() {
			return exit.Timeout, "timeout"
		}
		return exit.Tool, "network"
	}
	return exit.Tool, "internal"
}

func classifyAPI(e *api.Error) (int, string) {
	switch {
	case e.Status == http.StatusNotFound:
		return exit.NotFound, "not_found"
	case e.Status == http.StatusForbidden:
		return exit.Tool, "auth"
	case e.Status == http.StatusRequestTimeout, e.Status == http.StatusGatewayTimeout:
		return exit.Timeout, "timeout"
	case e.NotJSON:
		return exit.Tool, "network"
	}
	return exit.NotApplied, "api"
}

const unauthorizedHint = "токен истёк или отозван — выпустите новый в /settings и выполните `finances-kai login`"

// fail печатает отказ и возвращает код выхода. Текст — в stderr, как было;
// в режиме --json — конвертом в stdout.
func fail(p *output.Printer, err error) int {
	if errors.Is(err, errHelp) {
		return exit.OK
	}
	code, kind := classify(err)

	if p.JSON {
		msg := err.Error()
		if errors.Is(err, api.ErrUnauthorized) {
			msg += " — " + unauthorizedHint
		}
		p.Fail(code, kind, msg)
		return code
	}

	if code == exit.Interrupted {
		return code
	}
	var ce *cliError
	if errors.As(err, &ce) && ce.printed {
		return code
	}
	fmt.Fprintln(os.Stderr, "ошибка:", err)
	if errors.Is(err, api.ErrUnauthorized) {
		fmt.Fprintln(os.Stderr, "подсказка: "+unauthorizedHint)
	}
	return code
}
