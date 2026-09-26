package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Kaidstor/finances-kai/internal/api"
)

// Смысл CLI поверх curl — принимать имена, а не UUID. Здесь имена из флагов
// превращаются в идентификаторы; неоднозначность — ошибка, а не догадка.

type tagIndex struct {
	tags  []api.Tag
	byID  map[string]api.Tag
	loads bool
}

func (s *session) loadTags(ctx context.Context) (*tagIndex, error) {
	tags, err := s.client.Tags(ctx)
	if err != nil {
		return nil, err
	}
	idx := &tagIndex{tags: tags, byID: make(map[string]api.Tag, len(tags)), loads: true}
	for _, t := range tags {
		idx.byID[t.ID] = t
	}
	return idx, nil
}

// path отдаёт «родитель / тег» для подтегов — иначе одноимённые подтеги разных
// родителей неразличимы в выводе и в сообщении о неоднозначности.
func (i *tagIndex) path(t api.Tag) string {
	if t.ParentID == nil {
		return t.Name
	}
	if parent, ok := i.byID[*t.ParentID]; ok {
		return parent.Name + " / " + t.Name
	}
	return t.Name
}

func (i *tagIndex) resolve(query string) (api.Tag, error) {
	names := make([]string, 0, len(i.tags))
	for _, t := range i.tags {
		names = append(names, i.path(t))
	}
	matches := match(query, names)

	switch len(matches) {
	case 1:
		return i.tags[matches[0]], nil
	case 0:
		return api.Tag{}, notFound("тег %q не найден; есть: %s", query, preview(names))
	default:
		var found []string
		for _, m := range matches {
			found = append(found, i.path(i.tags[m]))
		}
		return api.Tag{}, ambiguous("тег %q подходит к нескольким: %s — уточните",
			query, strings.Join(found, ", "))
	}
}

func (i *tagIndex) resolveAll(queries []string) ([]string, error) {
	ids := make([]string, 0, len(queries))
	for _, q := range queries {
		t, err := i.resolve(q)
		if err != nil {
			return nil, err
		}
		ids = append(ids, t.ID)
	}
	return ids, nil
}

type cpIndex struct{ items []api.Counterparty }

func (s *session) loadCounterparties(ctx context.Context) (*cpIndex, error) {
	items, err := s.client.Counterparties(ctx)
	if err != nil {
		return nil, err
	}
	return &cpIndex{items: items}, nil
}

func (i *cpIndex) resolve(query string) (api.Counterparty, error) {
	names := make([]string, 0, len(i.items))
	for _, c := range i.items {
		names = append(names, c.Name)
	}
	matches := match(query, names)

	switch len(matches) {
	case 1:
		return i.items[matches[0]], nil
	case 0:
		return api.Counterparty{}, notFound("контрагент %q не найден; есть: %s", query, preview(names))
	default:
		var found []string
		for _, m := range matches {
			found = append(found, i.items[m].Name)
		}
		return api.Counterparty{}, ambiguous("контрагент %q подходит к нескольким: %s — уточните",
			query, strings.Join(found, ", "))
	}
}

func (i *cpIndex) resolveAll(queries []string) ([]string, error) {
	ids := make([]string, 0, len(queries))
	for _, q := range queries {
		c, err := i.resolve(q)
		if err != nil {
			return nil, err
		}
		ids = append(ids, c.ID)
	}
	return ids, nil
}

// match ищет по именам: сначала точное совпадение без учёта регистра, и только
// если его нет — подстроку. Иначе «еда» никогда не выбрала бы себя при наличии
// «еда вне дома».
func match(query string, names []string) []int {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	var exact, partial []int
	for i, name := range names {
		lower := strings.ToLower(name)
		switch {
		case lower == q:
			exact = append(exact, i)
		case strings.Contains(lower, q):
			partial = append(partial, i)
		}
		// Подтег можно назвать хвостом пути: «Еда / кафе» находится по «кафе».
		if _, child, ok := strings.Cut(lower, " / "); ok && child == q {
			exact = append(exact, i)
		}
	}
	if len(exact) > 0 {
		return dedup(exact)
	}
	return partial
}

func dedup(in []int) []int {
	seen := map[int]bool{}
	out := in[:0:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// preview показывает несколько доступных имён — полный список в ошибке не нужен.
func preview(names []string) string {
	if len(names) == 0 {
		return "(список пуст)"
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if len(sorted) > 12 {
		return strings.Join(sorted[:12], ", ") + fmt.Sprintf(" и ещё %d", len(sorted)-12)
	}
	return strings.Join(sorted, ", ")
}
