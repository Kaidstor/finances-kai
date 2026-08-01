package command

import (
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/Kaidstor/finances-kai/internal/config"
)

// exportFlags повторяет набор флагов cmdExport — нужен, чтобы mergePreset
// проверялся ровно на тех же именах и значениях по умолчанию.
func exportFlags(args []string) (*flag.FlagSet, stringList, stringList) {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("period", "month", "")
	fs.String("from", "", "")
	fs.String("to", "", "")
	fs.String("type", "", "")
	var tags, cps stringList
	fs.Var(&tags, "tag", "")
	fs.Var(&cps, "cp", "")
	if err := fs.Parse(args); err != nil {
		panic(err)
	}
	return fs, tags, cps
}

func TestMergePreset(t *testing.T) {
	saved := config.ExportPreset{
		Period: "year", Type: "expense", Tags: []string{"аренда"},
	}

	t.Run("без флагов набор остаётся как есть", func(t *testing.T) {
		fs, tags, cps := exportFlags(nil)
		got := mergePreset(saved, fs, tags, cps)
		if got.Period != "year" || got.Type != "expense" || len(got.Tags) != 1 {
			t.Errorf("набор изменился: %+v", got)
		}
	})

	t.Run("период запуска перекрывает сохранённый", func(t *testing.T) {
		fs, tags, cps := exportFlags([]string{"--period", "month"})
		got := mergePreset(saved, fs, tags, cps)
		if got.Period != "month" {
			t.Errorf("период = %q, want month", got.Period)
		}
		if got.Type != "expense" || len(got.Tags) != 1 {
			t.Errorf("остальные фильтры потерялись: %+v", got)
		}
	})

	t.Run("явные границы перекрывают сохранённый период", func(t *testing.T) {
		fs, tags, cps := exportFlags([]string{"--from", "2026-01-01", "--to", "2026-03-31"})
		got := mergePreset(saved, fs, tags, cps)
		if got.From != "2026-01-01" || got.To != "2026-03-31" {
			t.Errorf("границы = %q..%q", got.From, got.To)
		}
	})

	// Главная ловушка: у сохранённого набора границы приоритетнее периода,
	// поэтому переданный --period без их сброса не подействовал бы.
	t.Run("период запуска сбрасывает сохранённые границы", func(t *testing.T) {
		withRange := config.ExportPreset{From: "2026-01-01", To: "2026-03-31", Type: "expense"}
		fs, tags, cps := exportFlags([]string{"--period", "year"})
		got := mergePreset(withRange, fs, tags, cps)
		if got.From != "" || got.To != "" {
			t.Errorf("границы не сброшены: %q..%q", got.From, got.To)
		}
		if got.Period != "year" {
			t.Errorf("период = %q, want year", got.Period)
		}
	})

	t.Run("теги запуска заменяют сохранённые", func(t *testing.T) {
		fs, tags, cps := exportFlags([]string{"--tag", "еда", "--tag", "кафе"})
		got := mergePreset(saved, fs, tags, cps)
		if strings.Join(got.Tags, ",") != "еда,кафе" {
			t.Errorf("теги = %v", got.Tags)
		}
	})

	t.Run("пустой набор получает месяц по умолчанию", func(t *testing.T) {
		fs, tags, cps := exportFlags(nil)
		got := mergePreset(config.ExportPreset{}, fs, tags, cps)
		if got.Period != "month" {
			t.Errorf("период = %q, want month", got.Period)
		}
	})
}

func TestDescribePreset(t *testing.T) {
	cases := []struct {
		name     string
		preset   config.ExportPreset
		contains []string
	}{
		{
			name:     "фильтры перечисляются",
			preset:   config.ExportPreset{Type: "expense", Tags: []string{"аренда"}},
			contains: []string{"расходы", "аренда"},
		},
		{
			name:     "явные границы вместо периода",
			preset:   config.ExportPreset{From: "2026-01-01", To: "2026-03-31"},
			contains: []string{"2026-01-01", "2026-03-31"},
		},
		{
			name:     "пустой набор",
			preset:   config.ExportPreset{Period: "month"},
			contains: []string{"без фильтров"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := describePreset(c.preset)
			for _, want := range c.contains {
				if !strings.Contains(got, want) {
					t.Errorf("описание %q не содержит %q", got, want)
				}
			}
		})
	}
}
