// Package output — единственное место, где что-то печатается в stdout:
// таблицы, суммы, даты, цвета и режим --json.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var symbols = map[string]string{"RUB": "₽", "USD": "$", "KZT": "₸"}

// Currency возвращает символ валюты; неизвестный код показываем как есть.
func Currency(code string) string {
	if s, ok := symbols[code]; ok {
		return s
	}
	return code
}

// Amount форматирует сумму как в приложении: разряды неразрывными пробелами,
// запятая как десятичный разделитель, символ валюты в конце.
func Amount(raw, currency string) string {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw + " " + Currency(currency)
	}
	return group(v) + " " + Currency(currency)
}

func group(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	intPart, frac, _ := strings.Cut(s, ".")

	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return sign + b.String() + "," + frac
}

// Date приводит дату платежа к YYYY-MM-DD: сервер отдаёт её timestamp-строкой,
// а разделителем бывает и "T", и пробел.
func Date(raw string) string {
	if i := strings.IndexAny(raw, "T "); i > 0 {
		return raw[:i]
	}
	return raw
}

// JSON печатает значение как отступленный JSON — режим --json у всех команд.
func JSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Table печатает шапку и строки, выравнивая колонки по ширине содержимого.
// Ширина считается в рунах: для кириллицы это верно, для эмодзи в иконках
// контрагентов даёт погрешность в один столбец — на читаемость не влияет.
type Table struct {
	head  []string
	rows  [][]string
	right map[int]bool
}

func NewTable(head ...string) *Table {
	return &Table{head: head, right: map[int]bool{}}
}

// RightAlign помечает колонки, которые выравниваются по правому краю (суммы).
func (t *Table) RightAlign(cols ...int) *Table {
	for _, c := range cols {
		t.right[c] = true
	}
	return t
}

func (t *Table) Add(cells ...string) { t.rows = append(t.rows, cells) }

func (t *Table) Len() int { return len(t.rows) }

func (t *Table) Render(w io.Writer) {
	widths := make([]int, len(t.head))
	for i, h := range t.head {
		widths[i] = width(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) && width(cell) > widths[i] {
				widths[i] = width(cell)
			}
		}
	}

	// Таблица из одних значений (например, итоги) заводится с пустой шапкой —
	// печатать её значит выводить пустую строку.
	if header := line(t.head, widths, t.right); strings.TrimSpace(header) != "" {
		fmt.Fprintln(w, Dim(header))
	}
	for _, row := range t.rows {
		fmt.Fprintln(w, line(row, widths, t.right))
	}
}

func line(cells []string, widths []int, right map[int]bool) string {
	parts := make([]string, 0, len(cells))
	for i, cell := range cells {
		if i >= len(widths) {
			parts = append(parts, cell)
			continue
		}
		pad := strings.Repeat(" ", widths[i]-width(cell))
		if right[i] {
			parts = append(parts, pad+cell)
		} else {
			parts = append(parts, cell+pad)
		}
	}
	// Хвостовые пробелы последней колонки мешают копированию из терминала.
	return strings.TrimRight(strings.Join(parts, "  "), " ")
}

// width считает видимую ширину в знакоместах терминала. Две поправки к
// «просто число рун»: escape-последовательности цвета невидимы, а эмодзи и
// иероглифы занимают два знакоместа — без этого колонка с иконкой контрагента
// разъезжается.
func width(s string) int {
	visible := 0
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case inEscape:
		case isWide(r):
			visible += 2
		default:
			visible++
		}
	}
	return visible
}

// isWide покрывает диапазоны, которые реально встречаются в данных: эмодзи в
// иконках контрагентов и CJK. Полная таблица East Asian Width сюда не нужна.
//
// Символы из блока U+2600 (☕, ✈) намеренно не считаются широкими: их ширина
// в стандарте «неоднозначная», и терминалы рисуют их по-разному.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // хангыль-джамо
		r >= 0x2E80 && r <= 0xA4CF, // CJK, кана, радикалы
		r >= 0xAC00 && r <= 0xD7A3, // хангыль
		r >= 0xF900 && r <= 0xFAFF, // CJK-совместимость
		r >= 0xFE30 && r <= 0xFE6F, // CJK-формы
		r >= 0xFF00 && r <= 0xFF60, // полноширинные формы
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F9FF, // эмодзи: пиктограммы, смайлы, транспорт
		r >= 0x1FA70 && r <= 0x1FAFF, // эмодзи-дополнения
		r >= 0x20000 && r <= 0x3FFFD: // CJK-расширения
		return true
	}
	return false
}
