package keyring

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeSec кладёт в PATH скрипт sec, который отвечает на `sec get proj/KEY`.
func fakeSec(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sec"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FINANCES_KAI_HOME", t.TempDir())
}

func TestGetOrder(t *testing.T) {
	fakeSec(t, `[ "$1 $2" = "get proj/KEY" ] && echo tok-from-sec`)

	t.Setenv("FINANCES_KAI_TOKEN", "tok-from-env")
	tok, err := Get("default", "proj/KEY")
	if err != nil || tok.Source != SourceEnv || tok.Value != "tok-from-env" {
		t.Fatalf("env должен побеждать token_ref: %+v, %v", tok, err)
	}

	t.Setenv("FINANCES_KAI_TOKEN", "")
	tok, err = Get("default", "proj/KEY")
	if err != nil || tok.Source != SourceSec || tok.Value != "tok-from-sec" || tok.Ref != "proj/KEY" {
		t.Fatalf("token_ref: %+v, %v", tok, err)
	}
	if tok.Label() != "sec proj/KEY" {
		t.Errorf("Label = %q", tok.Label())
	}
}

func TestGetSecFailureIsError(t *testing.T) {
	fakeSec(t, `echo "нет такого ключа" >&2; exit 3`)
	t.Setenv("FINANCES_KAI_TOKEN", "")

	// Файловый токен рядом не должен подставиться вместо сломанного token_ref.
	path := filePath("default")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tok-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tok, err := Get("default", "proj/KEY")
	if err == nil {
		t.Fatalf("ожидалась ошибка sec, получен токен из %s", tok.Source)
	}
}

func TestMask(t *testing.T) {
	if got := Mask("abcdefghij"); got != "ab…ij (10 символов)" {
		t.Errorf("Mask = %q", got)
	}
	if got := Mask("abc"); got != "(3 символов)" {
		t.Errorf("короткий Mask = %q", got)
	}
}
