package output

import (
	"os"
	"sync"
)

var colorEnabled = sync.OnceValue(func() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	// Вывод в пайп или файл — красить нечего: escape-коды уедут в данные.
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
})

func paint(code, s string) string {
	if s == "" || !colorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func Green(s string) string  { return paint("32", s) }
func Red(s string) string    { return paint("31", s) }
func Yellow(s string) string { return paint("33", s) }
func Cyan(s string) string   { return paint("36", s) }
func Bold(s string) string   { return paint("1", s) }
func Dim(s string) string    { return paint("2", s) }
