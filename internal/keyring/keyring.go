// Package keyring хранит API-токены профилей. Порядок поиска:
// env FINANCES_KAI_TOKEN → системное хранилище ОС → файл в каталоге конфигурации.
//
// Системное хранилище дёргается через штатные утилиты (`security` на macOS,
// `secret-tool` на Linux), поэтому платформенных сборочных тегов здесь нет —
// только проверка runtime.GOOS и честный откат на файл, если утилиты нет.
package keyring

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Kaidstor/finances-next/cli/internal/config"
)

const service = "finances-kai"

// ErrNotFound — токена нет ни в одном из источников.
var ErrNotFound = errors.New("токен не найден")

// Source описывает, откуда взят токен — для `doctor` и сообщений об ошибках.
type Source string

const (
	SourceEnv   Source = "переменная FINANCES_KAI_TOKEN"
	SourceOS    Source = "системное хранилище"
	SourceFile  Source = "файл"
	SourceNone  Source = "—"
	backendNone        = ""
)

// Get возвращает токен профиля и источник, из которого он взят.
func Get(profile string) (string, Source, error) {
	if t := strings.TrimSpace(os.Getenv("FINANCES_KAI_TOKEN")); t != "" {
		return t, SourceEnv, nil
	}
	if t, err := osRead(profile); err == nil && t != "" {
		return t, SourceOS, nil
	}
	raw, err := os.ReadFile(filePath(profile))
	if err == nil {
		if t := strings.TrimSpace(string(raw)); t != "" {
			return t, SourceFile, nil
		}
	}
	return "", SourceNone, ErrNotFound
}

// Set кладёт токен в системное хранилище, а если его нет — в файл 0600.
// Возвращает источник, куда фактически записал.
func Set(profile, token string) (Source, error) {
	if err := osWrite(profile, token); err == nil {
		// Файловая копия от прошлого запуска сделала бы `doctor` лживым:
		// он показал бы системное хранилище, а рядом лежал бы старый токен.
		os.Remove(filePath(profile))
		return SourceOS, nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath(profile)), 0o700); err != nil {
		return SourceNone, fmt.Errorf("создаю каталог токенов: %w", err)
	}
	if err := os.WriteFile(filePath(profile), []byte(token+"\n"), 0o600); err != nil {
		return SourceNone, fmt.Errorf("пишу токен: %w", err)
	}
	return SourceFile, nil
}

// Delete убирает токен профиля отовсюду, куда мы могли его положить.
func Delete(profile string) {
	osDelete(profile)
	os.Remove(filePath(profile))
}

// BackendName — человекочитаемое имя системного хранилища или пустая строка,
// если на этой машине его нет.
func BackendName() string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("security"); err == nil {
			return "Keychain"
		}
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err == nil {
			return "libsecret"
		}
	}
	return backendNone
}

func filePath(profile string) string {
	return filepath.Join(config.Dir(), "tokens", profile)
}

func osRead(profile string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("security", "find-generic-password",
			"-s", service, "-a", profile, "-w").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		out, err := exec.Command("secret-tool", "lookup",
			"service", service, "account", profile).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", ErrNotFound
}

func osWrite(profile, token string) error {
	switch runtime.GOOS {
	case "darwin":
		// Команда уходит в `security -i` через stdin, а не в argv: иначе токен
		// был бы виден в ps любому процессу пользователя.
		// -U обновляет существующую запись; без него security падает на дубле.
		cmd := exec.Command("security", "-i")
		cmd.Stdin = strings.NewReader(fmt.Sprintf(
			"add-generic-password -U -s %q -a %q -w %q\n", service, profile, token))
		return cmd.Run()
	case "linux":
		cmd := exec.Command("secret-tool", "store", "--label", service+" "+profile,
			"service", service, "account", profile)
		cmd.Stdin = bytes.NewBufferString(token)
		return cmd.Run()
	}
	return errors.New("системного хранилища нет на этой платформе")
}

func osDelete(profile string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("security", "delete-generic-password",
			"-s", service, "-a", profile).Run()
	case "linux":
		_ = exec.Command("secret-tool", "clear",
			"service", service, "account", profile).Run()
	}
}
