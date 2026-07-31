package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Список типов должен совпадать с ALLOWED_MIME_TYPES в
// lib/services/file-storage.ts: сервер сверяет присланный Content-Type с ним,
// и расхождение даст 400 уже после отправки файла.
func TestUploadTypesMatchServerWhitelist(t *testing.T) {
	serverMimeTypes := map[string]bool{
		"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true,
		"application/pdf": true, "text/plain": true, "text/csv": true,
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
	}
	serverExtensions := []string{
		".jpg", ".jpeg", ".png", ".gif", ".webp",
		".pdf", ".txt", ".csv", ".doc", ".docx", ".xls", ".xlsx",
	}

	for _, ext := range serverExtensions {
		ct, ok := uploadTypes[ext]
		if !ok {
			t.Errorf("расширение %s сервер принимает, а CLI отвергнет", ext)
			continue
		}
		if !serverMimeTypes[ct] {
			t.Errorf("для %s CLI шлёт %q, сервер такой тип не принимает", ext, ct)
		}
	}
	if len(uploadTypes) != len(serverExtensions) {
		t.Errorf("в CLI %d расширений, у сервера %d", len(uploadTypes), len(serverExtensions))
	}
}

func TestCheckUploadable(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "чек.PNG") // регистр расширения не должен мешать
	if err := os.WriteFile(good, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ct, err := checkUploadable(good)
	if err != nil {
		t.Fatalf("checkUploadable: %v", err)
	}
	if ct != "image/png" {
		t.Errorf("тип = %q, want image/png", ct)
	}

	bad := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(bad, []byte("#!/bin/sh"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := checkUploadable(bad); err == nil {
		t.Error("исполняемое расширение должно отвергаться")
	}

	if _, err := checkUploadable(dir); err == nil {
		t.Error("каталог должен отвергаться")
	}
	if _, err := checkUploadable(filepath.Join(dir, "нет-такого.png")); err == nil {
		t.Error("отсутствующий файл должен отвергаться")
	}

	big := filepath.Join(dir, "big.pdf")
	if err := os.WriteFile(big, make([]byte, maxFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = checkUploadable(big)
	if err == nil || !strings.Contains(err.Error(), "максимум") {
		t.Errorf("файл больше лимита должен отвергаться, получено %v", err)
	}
}
