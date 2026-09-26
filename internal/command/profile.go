package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Kaidstor/finances-kai/internal/api"
	"github.com/Kaidstor/finances-kai/internal/config"
	"github.com/Kaidstor/finances-kai/internal/exit"
	"github.com/Kaidstor/finances-kai/internal/keyring"
	"github.com/Kaidstor/finances-kai/internal/output"
)

const loginHelp = `finances-kai login — сохранить API-токен профиля

Токен выпускается в приложении: /settings → «API Токены». Он читается со
stdin или из FINANCES_KAI_TOKEN — в аргументах командной строки его передавать
не нужно, оттуда он виден в ps и остаётся в истории шелла.

С --token-ref токен берётся из sec (sec get <ссылка>), в профиль сохраняется
только ссылка, системное хранилище не трогается.

  finances-kai login --url https://finances.example.com
  finances-kai login --profile local --url http://localhost:3000
  pbpaste | finances-kai login --url https://finances.example.com
  finances-kai login --url https://finances.example.com --token-ref finances/API_TOKEN`

func cmdLogin(ctx context.Context, pr *output.Printer, args []string) error {
	fs, profile := newFlagSet(pr, "login", loginHelp)
	rawURL := fs.String("url", "", "адрес приложения")
	tokenRef := fs.String("token-ref", "", "ссылка на токен в sec вида <проект>/<KEY>")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	name := *profile
	if name == "" {
		name = os.Getenv("FINANCES_KAI_PROFILE")
	}
	if name == "" {
		name = cfg.Current
	}
	if name == "" {
		name = "default"
	}

	p := cfg.Profiles[name]
	if *rawURL != "" {
		p.URL = strings.TrimRight(*rawURL, "/")
	}
	if p.URL == "" {
		return usageErr("не задан адрес приложения: добавьте --url")
	}

	var token string
	if *tokenRef != "" {
		if token, err = keyring.FromSec(*tokenRef); err != nil {
			return authErr("token_ref %s: %s", *tokenRef, err)
		}
	} else if token, err = readToken(); err != nil {
		return err
	}

	// Проверяем токен до сохранения: иначе профиль остался бы с мусором,
	// а ошибку человек увидел бы только на следующей команде.
	user, err := api.New(p.URL, token).Me(ctx)
	if err != nil {
		return fmt.Errorf("токен не подошёл к %s: %w", p.URL, err)
	}
	p.Currency = user.DefaultCurrency
	// token_ref читается раньше хранилища: без сброса только что сохранённый
	// токен молча не использовался бы.
	p.TokenRef = *tokenRef

	cfg.Profiles[name] = p
	cfg.Current = name
	if err := cfg.Save(); err != nil {
		return configErr(err)
	}

	label := "sec " + *tokenRef
	source := keyring.SourceSec
	if *tokenRef == "" {
		source, err = keyring.Set(name, token)
		if err != nil {
			return configErr(err)
		}
		label = string(source)
		if source == keyring.SourceFile {
			pr.Warn("системного хранилища нет — токен лежит файлом с правами 0600")
		}
	}

	if pr.JSON {
		return pr.Data(map[string]string{
			"profile": name, "url": p.URL, "user": user.Email,
			"currency": user.DefaultCurrency, "token_source": label,
		})
	}
	fmt.Printf("%s профиль %s → %s\n", output.Green("сохранён"), output.Bold(name), p.URL)
	fmt.Printf("пользователь %s, базовая валюта %s\n", user.Email, output.Currency(user.DefaultCurrency))
	fmt.Printf("%s\n", output.Dim("токен: "+label))
	return nil
}

// readToken берёт токен из окружения или stdin, но не из argv.
func readToken() (string, error) {
	if t := strings.TrimSpace(os.Getenv("FINANCES_KAI_TOKEN")); t != "" {
		return t, nil
	}

	info, err := os.Stdin.Stat()
	if err == nil && info.Mode()&os.ModeCharDevice == 0 {
		raw, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && raw == "" {
			return "", usageErr("на stdin пусто: передайте токен туда или через FINANCES_KAI_TOKEN")
		}
		if t := strings.TrimSpace(raw); t != "" {
			return t, nil
		}
		return "", usageErr("на stdin пусто: передайте токен туда или через FINANCES_KAI_TOKEN")
	}

	fmt.Fprint(os.Stderr, "API-токен (/settings → «API Токены»): ")
	raw, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && raw == "" {
		return "", err
	}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", usageErr("пустой токен")
	}
	return token, nil
}

const profileHelp = `finances-kai profile [use|add|rm] — профили подключения

  finances-kai profile                      список
  finances-kai profile use local            переключить текущий
  finances-kai profile add local http://localhost:3000
  finances-kai profile rm local             удалить вместе с токеном`

func cmdProfile(ctx context.Context, pr *output.Printer, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "use":
			pr.Command = "profile use"
			return profileUse(pr, args[1:])
		case "add":
			pr.Command = "profile add"
			return profileAdd(pr, args[1:])
		case "rm", "delete":
			pr.Command = "profile rm"
			return profileRemove(pr, args[1:])
		}
	}

	fs, _ := newFlagSet(pr, "profile", profileHelp)
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if pr.JSON {
		return pr.Data(cfg)
	}
	if len(cfg.Profiles) == 0 {
		fmt.Println("Профилей нет. Заведите первый: finances-kai login --url <адрес>")
		return nil
	}

	t := output.NewTable("", "ПРОФИЛЬ", "АДРЕС", "ВАЛЮТА", "ТОКЕН")
	for _, name := range cfg.Names() {
		p := cfg.Profiles[name]
		marker := " "
		if name == cfg.Current {
			marker = output.Green("*")
		}
		tok, err := keyring.Get(name, p.TokenRef)
		token := tok.Label()
		if err != nil {
			token = output.Yellow("нет")
		}
		t.Add(marker, name, p.URL, output.Currency(p.Currency), output.Dim(token))
	}
	t.Render(os.Stdout)
	return nil
}

func profileArgs(want int, args []string) error {
	if len(args) != want {
		fmt.Fprintln(os.Stderr, profileHelp)
		return &cliError{code: exit.Tool, kind: "usage", printed: true,
			err: fmt.Errorf("ожидалось аргументов: %d, получено %d", want, len(args))}
	}
	return nil
}

func profileUse(pr *output.Printer, args []string) error {
	if err := profileArgs(1, args); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[args[0]]; !ok {
		return notFound("профиль %q не найден; есть: %s", args[0], strings.Join(cfg.Names(), ", "))
	}
	cfg.Current = args[0]
	if err := cfg.Save(); err != nil {
		return configErr(err)
	}
	if pr.JSON {
		return pr.Data(map[string]string{"current": args[0]})
	}
	fmt.Printf("текущий профиль: %s\n", output.Bold(args[0]))
	return nil
}

func profileAdd(pr *output.Printer, args []string) error {
	if err := profileArgs(2, args); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	p := cfg.Profiles[args[0]]
	p.URL = strings.TrimRight(args[1], "/")
	cfg.Profiles[args[0]] = p
	if cfg.Current == "" {
		cfg.Current = args[0]
	}
	if err := cfg.Save(); err != nil {
		return configErr(err)
	}
	if pr.JSON {
		return pr.Data(map[string]string{"profile": args[0], "url": p.URL})
	}
	fmt.Printf("%s профиль %s → %s\n", output.Green("сохранён"), args[0], p.URL)
	fmt.Printf("токен: finances-kai login --profile %s\n", args[0])
	return nil
}

func profileRemove(pr *output.Printer, args []string) error {
	if err := profileArgs(1, args); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[args[0]]; !ok {
		return notFound("профиль %q не найден", args[0])
	}
	delete(cfg.Profiles, args[0])
	if cfg.Current == args[0] {
		cfg.Current = ""
		if names := cfg.Names(); len(names) > 0 {
			cfg.Current = names[0]
		}
	}
	if err := cfg.Save(); err != nil {
		return configErr(err)
	}
	keyring.Delete(args[0])
	if pr.JSON {
		return pr.Data(deleted{ID: args[0], Name: args[0], Deleted: true})
	}
	fmt.Printf("%s профиль %s\n", output.Yellow("удалён"), args[0])
	return nil
}

const doctorHelp = `finances-kai doctor — откуда прочитаны настройки, источник токена с маской
и связь с приложением одним запросом`

type doctorData struct {
	Config       string `json:"config"`
	ConfigExists bool   `json:"config_exists"`
	Profile      string `json:"profile"`
	URL          string `json:"url"`
	Backend      string `json:"backend"`
	TokenSource  string `json:"token_source"`
	TokenMask    string `json:"token_mask"`
	User         string `json:"user"`
	Currency     string `json:"currency"`
}

func cmdDoctor(ctx context.Context, pr *output.Printer, args []string) error {
	fs, profile := newFlagSet(pr, "doctor", doctorHelp)
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	line := func(label, value string) {
		if !pr.JSON {
			fmt.Printf("%-16s %s\n", label, value)
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	data := doctorData{Config: config.Path(), Backend: keyring.BackendName()}
	_, statErr := os.Stat(data.Config)
	data.ConfigExists = statErr == nil
	configLine := data.Config
	if !data.ConfigExists {
		configLine += output.Dim(" (файла нет)")
	}
	line("настройки", configLine)

	name, p, err := cfg.Resolve(*profile)
	if err != nil {
		return err
	}
	data.Profile, data.URL = name, p.URL
	line("профиль", output.Bold(name))
	line("адрес", p.URL)

	backend := data.Backend
	if backend == "" {
		backend = output.Yellow("нет, откат на файл")
	}
	line("хранилище", backend)

	token, err := keyring.Get(name, p.TokenRef)
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		line("токен", output.Red("не найден"))
		return authErr("выполните `finances-kai login --profile %s`", name)
	case err != nil:
		line("токен", output.Red("ошибка"))
		return authErr("%s", err)
	}
	data.TokenSource = tokenSourceID(token, name)
	data.TokenMask = keyring.Mask(token.Value)
	line("токен", token.Label()+" · "+data.TokenMask)

	switch {
	case token.Source == keyring.SourceFile:
		pr.Warn("токен лежит открытым текстом в %s — положите его в sec и выполните "+
			"`finances-kai login --token-ref <проект>/<KEY>`", keyring.FilePath(name))
	case token.Source == keyring.SourceEnv && p.TokenRef != "":
		pr.Warn("FINANCES_KAI_TOKEN перекрывает token_ref %s профиля", p.TokenRef)
	}

	user, err := api.New(p.URL, token.Value).Me(ctx)
	if err != nil {
		line("связь", output.Red("нет"))
		return fmt.Errorf("токен из %s: %w", token.Label(), err)
	}
	data.User, data.Currency = user.Email, user.DefaultCurrency
	line("связь", output.Green("есть"))
	line("пользователь", user.Email)
	line("базовая валюта", output.Currency(user.DefaultCurrency))
	return pr.Data(data)
}

// tokenSourceID — источник токена в машинном виде: env:FINANCES_KAI_TOKEN,
// sec:<ссылка>, keychain, libsecret, file:<путь>.
func tokenSourceID(t keyring.Token, profile string) string {
	switch t.Source {
	case keyring.SourceEnv:
		return "env:FINANCES_KAI_TOKEN"
	case keyring.SourceSec:
		return "sec:" + t.Ref
	case keyring.SourceOS:
		return strings.ToLower(keyring.BackendName())
	case keyring.SourceFile:
		return "file:" + keyring.FilePath(profile)
	}
	return string(t.Source)
}
