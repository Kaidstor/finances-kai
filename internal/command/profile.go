package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Kaidstor/finances-next/cli/internal/api"
	"github.com/Kaidstor/finances-next/cli/internal/config"
	"github.com/Kaidstor/finances-next/cli/internal/keyring"
	"github.com/Kaidstor/finances-next/cli/internal/output"
)

const loginHelp = `finances-kai login — сохранить API-токен профиля

Токен выпускается в приложении: /settings → «API Токены». Он читается со
stdin или из FINANCES_KAI_TOKEN — в аргументах командной строки его передавать
не нужно, оттуда он виден в ps и остаётся в истории шелла.

  finances-kai login --url https://finances.example.com
  finances-kai login --profile local --url http://localhost:3000
  pbpaste | finances-kai login --url https://finances.example.com`

func cmdLogin(ctx context.Context, args []string) error {
	fs, profile, _ := newFlagSet("login", loginHelp)
	rawURL := fs.String("url", "", "адрес приложения")
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	cfg, err := config.Load()
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
		return errors.New("не задан адрес приложения: добавьте --url")
	}

	token, err := readToken()
	if err != nil {
		return err
	}

	// Проверяем токен до сохранения: иначе профиль остался бы с мусором,
	// а ошибку человек увидел бы только на следующей команде.
	user, err := api.New(p.URL, token).Me(ctx)
	if err != nil {
		return fmt.Errorf("токен не подошёл к %s: %w", p.URL, err)
	}
	p.Currency = user.DefaultCurrency

	cfg.Profiles[name] = p
	cfg.Current = name
	if err := cfg.Save(); err != nil {
		return err
	}

	source, err := keyring.Set(name, token)
	if err != nil {
		return err
	}

	fmt.Printf("%s профиль %s → %s\n", output.Green("сохранён"), output.Bold(name), p.URL)
	fmt.Printf("пользователь %s, базовая валюта %s\n", user.Email, output.Currency(user.DefaultCurrency))
	fmt.Printf("%s\n", output.Dim("токен: "+string(source)))
	if source == keyring.SourceFile {
		fmt.Fprintln(os.Stderr, output.Yellow("системного хранилища нет — токен лежит файлом с правами 0600"))
	}
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
			return "", errors.New("на stdin пусто: передайте токен туда или через FINANCES_KAI_TOKEN")
		}
		if t := strings.TrimSpace(raw); t != "" {
			return t, nil
		}
		return "", errors.New("на stdin пусто: передайте токен туда или через FINANCES_KAI_TOKEN")
	}

	fmt.Fprint(os.Stderr, "API-токен (/settings → «API Токены»): ")
	raw, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && raw == "" {
		return "", err
	}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("пустой токен")
	}
	return token, nil
}

const profileHelp = `finances-kai profile [use|add|rm] — профили подключения

  finances-kai profile                      список
  finances-kai profile use local            переключить текущий
  finances-kai profile add local http://localhost:3000
  finances-kai profile rm local             удалить вместе с токеном`

func cmdProfile(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "use":
			return profileUse(args[1:])
		case "add":
			return profileAdd(args[1:])
		case "rm", "delete":
			return profileRemove(args[1:])
		}
	}

	fs, _, asJSON := newFlagSet("profile", profileHelp)
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *asJSON {
		return output.JSON(cfg)
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
		_, source, err := keyring.Get(name)
		token := string(source)
		if err != nil {
			token = output.Yellow("нет")
		}
		t.Add(marker, name, p.URL, output.Currency(p.Currency), output.Dim(token))
	}
	t.Render(os.Stdout)
	return nil
}

func profileUse(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, profileHelp)
		return errParsed
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[args[0]]; !ok {
		return fmt.Errorf("профиль %q не найден; есть: %s", args[0], strings.Join(cfg.Names(), ", "))
	}
	cfg.Current = args[0]
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("текущий профиль: %s\n", output.Bold(args[0]))
	return nil
}

func profileAdd(args []string) error {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, profileHelp)
		return errParsed
	}
	cfg, err := config.Load()
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
		return err
	}
	fmt.Printf("%s профиль %s → %s\n", output.Green("сохранён"), args[0], p.URL)
	fmt.Printf("токен: finances-kai login --profile %s\n", args[0])
	return nil
}

func profileRemove(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, profileHelp)
		return errParsed
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[args[0]]; !ok {
		return fmt.Errorf("профиль %q не найден", args[0])
	}
	delete(cfg.Profiles, args[0])
	if cfg.Current == args[0] {
		cfg.Current = ""
		if names := cfg.Names(); len(names) > 0 {
			cfg.Current = names[0]
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	keyring.Delete(args[0])
	fmt.Printf("%s профиль %s\n", output.Yellow("удалён"), args[0])
	return nil
}

const doctorHelp = `finances-kai doctor — проверить профиль, токен и связь с приложением`

func cmdDoctor(ctx context.Context, args []string) error {
	fs, profile, _ := newFlagSet("doctor", doctorHelp)
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name, p, err := cfg.Resolve(*profile)
	if err != nil {
		return err
	}

	fmt.Printf("профиль          %s\n", output.Bold(name))
	fmt.Printf("адрес            %s\n", p.URL)

	backend := keyring.BackendName()
	if backend == "" {
		backend = output.Yellow("нет, откат на файл")
	}
	fmt.Printf("хранилище        %s\n", backend)

	token, source, err := keyring.Get(name)
	if err != nil {
		fmt.Printf("токен            %s\n", output.Red("не найден"))
		return fmt.Errorf("выполните `finances-kai login --profile %s`", name)
	}
	fmt.Printf("токен            %s\n", string(source))

	user, err := api.New(p.URL, token).Me(ctx)
	if err != nil {
		fmt.Printf("связь            %s\n", output.Red("нет"))
		return err
	}
	fmt.Printf("связь            %s\n", output.Green("есть"))
	fmt.Printf("пользователь     %s\n", user.Email)
	fmt.Printf("базовая валюта   %s\n", output.Currency(user.DefaultCurrency))
	return nil
}
