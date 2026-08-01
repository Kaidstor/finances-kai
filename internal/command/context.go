package command

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Kaidstor/finances-kai/internal/api"
	"github.com/Kaidstor/finances-kai/internal/config"
	"github.com/Kaidstor/finances-kai/internal/keyring"
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

// newFlagSet заводит набор флагов с общими для всех команд --profile и --json.
// Возвращает указатели, которые становятся валидными после Parse.
func newFlagSet(name, help string) (fs *flag.FlagSet, profile *string, asJSON *bool) {
	fs = flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, help)
		fmt.Fprintln(os.Stderr, "\nФлаги:")
		fs.PrintDefaults()
	}
	profile = fs.String("profile", "", "профиль вместо текущего")
	asJSON = fs.Bool("json", false, "машиночитаемый вывод")
	return fs, profile, asJSON
}

// open поднимает сессию: находит профиль и токен, собирает клиент.
func open(profileFlag string) (*session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	name, p, err := cfg.Resolve(profileFlag)
	if err != nil {
		return nil, err
	}
	if p.URL == "" {
		return nil, fmt.Errorf("у профиля %q не задан адрес; поправьте через `finances-kai profile add %s <url>`", name, name)
	}

	token, _, err := keyring.Get(name)
	if err != nil {
		return nil, fmt.Errorf("нет токена для профиля %q: выполните `finances-kai login --profile %s`", name, name)
	}

	return &session{
		client:   api.New(p.URL, token),
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
