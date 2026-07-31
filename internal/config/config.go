// Package config хранит профили подключения: имя → адрес приложения.
// Токен здесь не лежит — он в internal/keyring.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Profile struct {
	URL string `json:"url"`
	// Currency — валюта по умолчанию для `add`; пустая означает «спросить у
	// сервера», значение кешируется при login.
	Currency string `json:"currency,omitempty"`
}

// ExportPreset — сохранённый набор фильтров для export. Теги и контрагенты
// хранятся именами, а не идентификаторами: набор остаётся читаемым, переживает
// пересоздание тега и работает в любом профиле, где есть такие же имена.
type ExportPreset struct {
	Period         string   `json:"period,omitempty"`
	From           string   `json:"from,omitempty"`
	To             string   `json:"to,omitempty"`
	Type           string   `json:"type,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Counterparties []string `json:"counterparties,omitempty"`
}

type Config struct {
	Current  string                  `json:"current"`
	Profiles map[string]Profile      `json:"profiles"`
	Exports  map[string]ExportPreset `json:"exports,omitempty"`
}

// Dir — каталог конфигурации; FINANCES_KAI_HOME его переопределяет.
func Dir() string {
	if p := os.Getenv("FINANCES_KAI_HOME"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" && runtime.GOOS == "windows" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "finances-kai")
}

func path() string { return filepath.Join(Dir(), "config.json") }

func Load() (*Config, error) {
	cfg := &Config{Profiles: map[string]Profile{}}

	raw, err := os.ReadFile(path())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("читаю %s: %w", path(), err)
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("разбираю %s: %w", path(), err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return cfg, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return fmt.Errorf("создаю %s: %w", Dir(), err)
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	// Пишем через временный файл: обрыв на середине не должен оставлять
	// испорченный config.json, из которого CLI потом не поднимется.
	tmp := path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("пишу %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path()); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("сохраняю %s: %w", path(), err)
	}
	return nil
}

func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) ExportNames() []string {
	names := make([]string, 0, len(c.Exports))
	for name := range c.Exports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) SetExport(name string, p ExportPreset) {
	if c.Exports == nil {
		c.Exports = map[string]ExportPreset{}
	}
	c.Exports[name] = p
}

// Resolve возвращает профиль, который нужно использовать: явно названный,
// иначе current. FINANCES_KAI_PROFILE переопределяет current, а
// FINANCES_KAI_URL позволяет работать вообще без сохранённого профиля.
func (c *Config) Resolve(explicit string) (name string, p Profile, err error) {
	name = explicit
	if name == "" {
		name = os.Getenv("FINANCES_KAI_PROFILE")
	}
	if name == "" {
		name = c.Current
	}

	if url := os.Getenv("FINANCES_KAI_URL"); url != "" {
		if name == "" {
			name = "env"
		}
		p = c.Profiles[name]
		p.URL = url
		return name, p, nil
	}

	if name == "" {
		return "", Profile{}, errors.New("профиль не выбран: заведите его через `finances-kai login`")
	}
	p, ok := c.Profiles[name]
	if !ok {
		return "", Profile{}, fmt.Errorf("профиль %q не найден; есть: %s",
			name, strings.Join(c.Names(), ", "))
	}
	return name, p, nil
}
