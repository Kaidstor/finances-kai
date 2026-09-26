// Package keyring ищет API-токены профилей. Порядок поиска:
// env FINANCES_KAI_TOKEN → sec по token_ref профиля → системное хранилище ОС →
// файл в каталоге конфигурации (устаревший откат, doctor о нём предупреждает).
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

	"github.com/Kaidstor/finances-kai/internal/config"
)

const service = "finances-kai"

// ErrNotFound — токена нет ни в одном из источников.
var ErrNotFound = errors.New("токен не найден")

// Source описывает, откуда взят токен — для `doctor` и сообщений об ошибках.
type Source string

const (
	SourceEnv   Source = "переменная FINANCES_KAI_TOKEN"
	SourceSec   Source = "sec"
	SourceOS    Source = "системное хранилище"
	SourceFile  Source = "файл"
	SourceNone  Source = "—"
	backendNone        = ""
)

// Token — значение и откуда оно взято. Value наружу не печатается: для чата и
// логов есть Mask.
type Token struct {
	Value  string
	Source Source
	// Ref — ссылка в sec, если токен взят оттуда.
	Ref string
}

// Label — источник для человека: «sec finances/TOKEN», «системное хранилище».
func (t Token) Label() string {
	if t.Source == SourceSec {
		return "sec " + t.Ref
	}
	return string(t.Source)
}

// Get ищет токен профиля. tokenRef — token_ref профиля; пустой пропускает sec.
//
// Заданный token_ref, который sec не отдал, — ошибка, а не откат на хранилище:
// иначе молча подставился бы старый токен из Keychain, и doctor показал бы не
// тот источник, который настроен.
func Get(profile, tokenRef string) (Token, error) {
	if t := strings.TrimSpace(os.Getenv("FINANCES_KAI_TOKEN")); t != "" {
		return Token{Value: t, Source: SourceEnv}, nil
	}
	if tokenRef != "" {
		t, err := fromSec(tokenRef)
		if err != nil {
			return Token{Source: SourceNone}, fmt.Errorf("token_ref %s: %w", tokenRef, err)
		}
		return Token{Value: t, Source: SourceSec, Ref: tokenRef}, nil
	}
	if t, err := osRead(profile); err == nil && t != "" {
		return Token{Value: t, Source: SourceOS}, nil
	}
	raw, err := os.ReadFile(filePath(profile))
	if err == nil {
		if t := strings.TrimSpace(string(raw)); t != "" {
			return Token{Value: t, Source: SourceFile}, nil
		}
	}
	return Token{Source: SourceNone}, ErrNotFound
}

// FilePath — где лежит файловый откат токена профиля, для предупреждения doctor.
func FilePath(profile string) string { return filePath(profile) }

// Mask отдаёт представление токена, безопасное для чата и логов.
func Mask(value string) string {
	r := []rune(value)
	if len(r) < 8 {
		return fmt.Sprintf("(%d символов)", len(r))
	}
	return fmt.Sprintf("%s…%s (%d символов)", string(r[:2]), string(r[len(r)-2:]), len(r))
}

// FromSec читает значение по ссылке через `sec get`. Значение приходит в stdout
// дочернего процесса и в argv не попадает; в argv только сама ссылка.
func FromSec(ref string) (string, error) { return fromSec(ref) }

func fromSec(ref string) (string, error) {
	bin, err := exec.LookPath("sec")
	if err != nil {
		return "", fmt.Errorf("sec не найден в PATH: %w", err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(bin, "get", ref)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("sec get: %s", msg)
		}
		return "", fmt.Errorf("sec get: %w", err)
	}
	t := strings.TrimSpace(string(out))
	if t == "" {
		return "", errors.New("sec get вернул пустое значение")
	}
	return t, nil
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
