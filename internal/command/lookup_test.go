package command

import (
	"strings"
	"testing"

	"github.com/Kaidstor/finances-kai/internal/api"
)

func ptr(s string) *string { return &s }

func newTagIndex(tags []api.Tag) *tagIndex {
	idx := &tagIndex{tags: tags, byID: map[string]api.Tag{}}
	for _, t := range tags {
		idx.byID[t.ID] = t
	}
	return idx
}

func TestTagResolve(t *testing.T) {
	idx := newTagIndex([]api.Tag{
		{ID: "1", Name: "Еда"},
		{ID: "2", Name: "кафе", ParentID: ptr("1")},
		{ID: "3", Name: "Еда вне дома"},
		{ID: "4", Name: "Подписки"},
	})

	t.Run("точное совпадение выигрывает у подстроки", func(t *testing.T) {
		// «Еда» — подстрока «Еда вне дома»; без приоритета точного совпадения
		// запрос был бы неоднозначным и тег стало бы невозможно выбрать.
		got, err := idx.resolve("еда")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got.ID != "1" {
			t.Errorf("выбран тег %q, ожидался «Еда»", got.Name)
		}
	})

	t.Run("подтег по хвосту пути", func(t *testing.T) {
		got, err := idx.resolve("кафе")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got.ID != "2" {
			t.Errorf("выбран %q, ожидался «кафе»", got.Name)
		}
	})

	t.Run("подстрока, когда точного нет", func(t *testing.T) {
		got, err := idx.resolve("подпис")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got.ID != "4" {
			t.Errorf("выбран %q, ожидались «Подписки»", got.Name)
		}
	})

	t.Run("неоднозначность — ошибка со списком", func(t *testing.T) {
		_, err := idx.resolve("Е")
		if err == nil {
			t.Fatal("ожидалась ошибка неоднозначности")
		}
		if !strings.Contains(err.Error(), "подходит к нескольким") {
			t.Errorf("ошибка не про неоднозначность: %v", err)
		}
	})

	t.Run("не найдено", func(t *testing.T) {
		_, err := idx.resolve("бензин")
		if err == nil || !strings.Contains(err.Error(), "не найден") {
			t.Errorf("ожидалось «не найден», получено %v", err)
		}
	})
}

func TestTagPath(t *testing.T) {
	idx := newTagIndex([]api.Tag{
		{ID: "1", Name: "Еда"},
		{ID: "2", Name: "кафе", ParentID: ptr("1")},
		{ID: "3", Name: "сирота", ParentID: ptr("нет-такого")},
	})
	if got := idx.path(idx.byID["2"]); got != "Еда / кафе" {
		t.Errorf("path = %q, want «Еда / кафе»", got)
	}
	if got := idx.path(idx.byID["1"]); got != "Еда" {
		t.Errorf("path = %q, want «Еда»", got)
	}
	// Родителя нет в выдаче — показываем хотя бы имя, а не падаем.
	if got := idx.path(idx.byID["3"]); got != "сирота" {
		t.Errorf("path = %q, want «сирота»", got)
	}
}

func TestCounterpartyResolve(t *testing.T) {
	idx := &cpIndex{items: []api.Counterparty{
		{ID: "a", Name: "Азбука вкуса"},
		{ID: "b", Name: "Аптека"},
	}}

	got, err := idx.resolve("азбука")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.ID != "a" {
		t.Errorf("выбран %q", got.Name)
	}

	if _, err := idx.resolve("А"); err == nil {
		t.Error("ожидалась неоднозначность на «А»")
	}
}

func TestResolveAllStopsOnError(t *testing.T) {
	idx := newTagIndex([]api.Tag{{ID: "1", Name: "Еда"}})
	if _, err := idx.resolveAll([]string{"Еда", "нет-такого"}); err == nil {
		t.Error("ожидалась ошибка на втором теге")
	}
}
