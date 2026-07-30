package command

import (
	"flag"
	"io"
	"reflect"
	"testing"
)

// Пакет flag прекращает разбор на первом позиционном аргументе. Без
// предобработки `add -250 "кофе" --tag еда --paid` молча терял --tag и --paid,
// а описанием становилась строка «кофе --tag еда --paid». Тесты ниже держат
// именно этот случай.

func TestSplitLeading(t *testing.T) {
	cases := []struct {
		name           string
		in             []string
		wantPos, wantF []string
	}{
		{name: "только позиционные", in: []string{"кофе"}, wantPos: []string{"кофе"}},
		{name: "только флаги", in: []string{"--paid"}, wantF: []string{"--paid"}},
		{
			name:    "позиционные, затем флаги",
			in:      []string{"кофе", "--tag", "еда", "--paid"},
			wantPos: []string{"кофе"}, wantF: []string{"--tag", "еда", "--paid"},
		},
		{
			name:    "несколько слов до флагов",
			in:      []string{"обед", "в", "кафе", "--paid"},
			wantPos: []string{"обед", "в", "кафе"}, wantF: []string{"--paid"},
		},
		{
			name:    "-- отдаёт остаток как флаги",
			in:      []string{"описание", "--", "--не-флаг"},
			wantPos: []string{"описание"}, wantF: []string{"--не-флаг"},
		},
		{name: "пусто", in: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos, flags := splitLeading(c.in)
			if !equal(pos, c.wantPos) || !equal(flags, c.wantF) {
				t.Errorf("splitLeading(%v) = (%v, %v), want (%v, %v)",
					c.in, pos, flags, c.wantPos, c.wantF)
			}
		})
	}
}

func TestTakeAmount(t *testing.T) {
	amount, rest, err := takeAmount([]string{"-250", "кофе", "--paid"})
	if err != nil {
		t.Fatalf("takeAmount: %v", err)
	}
	if amount != "-250" {
		t.Errorf("сумма = %q, want -250", amount)
	}
	if !equal(rest, []string{"кофе", "--paid"}) {
		t.Errorf("остаток = %v", rest)
	}

	if _, _, err := takeAmount([]string{"кофе"}); err == nil {
		t.Error("ожидалась ошибка: первым аргументом должна быть сумма")
	}
}

// Ключевая регрессия: флаги за позиционным аргументом должны примениться.
func TestParseWithIDAppliesTrailingFlags(t *testing.T) {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")

	id, err := parseWithID(fs, []string{"11111111", "--yes"})
	if err != nil {
		t.Fatalf("parseWithID: %v", err)
	}
	if id != "11111111" {
		t.Errorf("id = %q", id)
	}
	if !*yes {
		t.Error("--yes после позиционного аргумента не применился")
	}
}

func TestParseWithIDRejectsExtra(t *testing.T) {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	if _, err := parseWithID(fs, []string{"a", "b"}); err == nil {
		t.Error("ожидалась ошибка на два id")
	}
	if _, err := parseWithID(fs, nil); err == nil {
		t.Error("ожидалась ошибка на отсутствующий id")
	}
}

func TestParseWithNameJoinsWordsAndAppliesFlags(t *testing.T) {
	fs := flag.NewFlagSet("cp add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	icon := fs.String("icon", "", "")

	name, err := parseWithName(fs, []string{"Азбука", "вкуса", "--icon", "🛒"})
	if err != nil {
		t.Fatalf("parseWithName: %v", err)
	}
	if name != "Азбука вкуса" {
		t.Errorf("имя = %q", name)
	}
	if *icon != "🛒" {
		t.Errorf("--icon не применился: %q", *icon)
	}
}

func TestCheckEnum(t *testing.T) {
	if err := checkEnum("--type", "", "income", "expense"); err != nil {
		t.Errorf("пустое значение должно проходить: %v", err)
	}
	if err := checkEnum("--type", "income", "income", "expense"); err != nil {
		t.Errorf("допустимое значение отвергнуто: %v", err)
	}
	if err := checkEnum("--type", "profit", "income", "expense"); err == nil {
		t.Error("ожидалась ошибка на недопустимое значение")
	}
}

func TestPlural(t *testing.T) {
	cases := map[int]string{
		1: "запись", 2: "записи", 4: "записи", 5: "записей",
		11: "записей", 12: "записей", 14: "записей",
		21: "запись", 22: "записи", 25: "записей", 101: "запись", 111: "записей",
	}
	for n, want := range cases {
		if got := plural(n, "запись", "записи", "записей"); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
