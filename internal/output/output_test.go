package output

import (
	"bytes"
	"strings"
	"testing"
)

// nbsp — разделитель разрядов. Такой же, как у toLocaleString("ru-RU") в
// приложении, и он же не даёт awk разорвать сумму на два поля.
const nbsp = " "

func TestAmount(t *testing.T) {
	cases := []struct {
		raw, currency, want string
	}{
		{"1234.56", "RUB", "1" + nbsp + "234,56 ₽"},
		{"-250.00", "RUB", "-250,00 ₽"},
		{"1000000", "USD", "1" + nbsp + "000" + nbsp + "000,00 $"},
		{"0", "KZT", "0,00 ₸"},
		{"12.5", "RUB", "12,50 ₽"},
		{"-1234567.89", "RUB", "-1" + nbsp + "234" + nbsp + "567,89 ₽"},
		// Неизвестный код валюты показываем как есть, а не прячем.
		{"10", "EUR", "10,00 EUR"},
		// Не число — отдаём как пришло, лишь бы не потерять значение.
		{"н/д", "RUB", "н/д ₽"},
	}
	for _, c := range cases {
		if got := Amount(c.raw, c.currency); got != c.want {
			t.Errorf("Amount(%q, %q) = %q, want %q", c.raw, c.currency, got, c.want)
		}
	}
}

func TestDate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2026-07-15T10:30:00.000Z", "2026-07-15"},
		{"2026-07-15 10:30:00", "2026-07-15"},
		{"2026-07-15", "2026-07-15"},
	}
	for _, c := range cases {
		if got := Date(c.in); got != c.want {
			t.Errorf("Date(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTableAlignsByRunes(t *testing.T) {
	tbl := NewTable("ТЕГ", "СУММА").RightAlign(1)
	tbl.Add("Еда", "100,00 ₽")
	tbl.Add("Подписки", "1"+nbsp+"200,00 ₽")

	var buf bytes.Buffer
	tbl.Render(&buf)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("ожидалось 3 строки, получено %d: %q", len(lines), buf.String())
	}
	// Кириллица шире в байтах, чем в рунах: выравнивание по len() разъехалось бы.
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, "Еда     ") && !strings.HasPrefix(l, "Подписки") {
			t.Errorf("колонка не выровнена: %q", l)
		}
	}
	// Суммы прижаты вправо — разряды должны совпасть по колонке.
	if !strings.HasSuffix(lines[1], "  100,00 ₽") {
		t.Errorf("сумма не выровнена вправо: %q", lines[1])
	}
}

func TestTableWidthIgnoresANSI(t *testing.T) {
	// Ширина считается по видимым символам: иначе escape-коды раздули бы колонку.
	if got := width("\x1b[32mЕда\x1b[0m"); got != 3 {
		t.Errorf("width = %d, want 3", got)
	}
}

func TestWidthCountsWideRunes(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"Еда", 3},             // кириллица — по знакоместу на букву
		{"🛒", 2},               // эмодзи занимает два
		{"🛒 Азбука вкуса", 15}, // 2 + пробел + 12
		{"日本", 4},              // CJK тоже широкий
		{"", 0},
	}
	for _, c := range cases {
		if got := width(c.in); got != c.want {
			t.Errorf("width(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// Колонка с эмодзи не должна съезжать относительно соседних строк.
func TestTableAlignsEmoji(t *testing.T) {
	tbl := NewTable("КОНТРАГЕНТ", "ID")
	tbl.Add("🛒 Азбука вкуса", "c-az")
	tbl.Add("Netflix", "c-nf")

	var buf bytes.Buffer
	tbl.Render(&buf)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	for _, l := range lines[1:] {
		name, _, ok := strings.Cut(l, "c-")
		if !ok {
			t.Fatalf("не нашёл id в строке %q", l)
		}
		if got := width(name); got != 17 { // 15 на колонку + 2 разделителя
			t.Errorf("отступ до id = %d в строке %q, want 17", got, l)
		}
	}
}
