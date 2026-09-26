package command

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Kaidstor/finances-kai/internal/api"
	"github.com/Kaidstor/finances-kai/internal/config"
	"github.com/Kaidstor/finances-kai/internal/keyring"
	"github.com/Kaidstor/finances-kai/internal/output"
)

// session — всё, что нужно команде: клиент, имя профиля и его настройки.
type session struct {
	client  *api.Client
	profile string
	cfg     *config.Config
	url     string
	// currency — валюта по умолчанию из профиля; пустая, если ещё не узнавали.
	currency string
}

// newFlagSet заводит набор флагов с общим для всех команд --profile. Указатель
// становится валидным после Parse. --json и --human сюда не входят: их
// вынимает Run из любого места argv.
//
// В режиме --json пакет flag молчит о неверном флаге — текст уходит в конверт;
// справка по -h печатается в stderr в обоих режимах.
func newFlagSet(p *output.Printer, name, help string) (fs *flag.FlagSet, profile *string) {
	fs = flag.NewFlagSet(name, flag.ContinueOnError)
	if p.JSON {
		fs.SetOutput(io.Discard)
	}
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, help)
		fmt.Fprintln(os.Stderr, "\nФлаги:")
		defer fs.SetOutput(fs.Output())
		fs.SetOutput(os.Stderr)
		fs.PrintDefaults()
	}
	profile = fs.String("profile", "", "профиль вместо текущего")
	return fs, profile
}

// loadConfig — config.Load с классом ошибки config.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	return cfg, configErr(err)
}

// open поднимает сессию: находит профиль и токен, собирает клиент.
func open(profileFlag string) (*session, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	name, p, err := cfg.Resolve(profileFlag)
	if err != nil {
		return nil, err
	}
	if p.URL == "" {
		return nil, configErr(fmt.Errorf("у профиля %q не задан адрес; поправьте через `finances-kai profile add %s <url>`", name, name))
	}

	token, err := keyring.Get(name, p.TokenRef)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, authErr("нет токена для профиля %q: выполните `finances-kai login --profile %s`", name, name)
	}
	if err != nil {
		return nil, authErr("%s", err)
	}

	return &session{
		client:   api.New(p.URL, token.Value),
		profile:  name,
		cfg:      cfg,
		url:      p.URL,
		currency: p.Currency,
	}, nil
}

// defaultCurrency отдаёт валюту для новых платежей: сначала из профиля, иначе
// спрашивает у сервера и запоминает, чтобы не ходить туда каждый раз.
func (s *session) defaultCurrency(ctx context.Context) string {
	if s.currency != "" {
		return s.currency
	}
	user, err := s.client.Me(ctx)
	if err != nil || user.DefaultCurrency == "" {
		return "RUB"
	}
	s.currency = user.DefaultCurrency
	if p, ok := s.cfg.Profiles[s.profile]; ok {
		p.Currency = user.DefaultCurrency
		s.cfg.Profiles[s.profile] = p
		_ = s.cfg.Save() // кеш, не критично
	}
	return s.currency
}

// stringList — повторяемый строковый флаг: --tag еда --tag кафе.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return fmt.Errorf("пустое значение")
	}
	*l = append(*l, v)
	return nil
}
