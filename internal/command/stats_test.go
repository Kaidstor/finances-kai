package command

import (
	"math"
	"testing"

	"github.com/Kaidstor/finances-kai/internal/api"
)

func payment(date, base string, tags ...string) api.Payment {
	p := api.Payment{
		Date: date, AmountInBaseCurrency: base, BaseCurrency: "RUB",
	}
	for _, name := range tags {
		p.Tags = append(p.Tags, api.Tag{ID: name, Name: name})
	}
	return p
}

func TestAggregateByTagExpenses(t *testing.T) {
	payments := []api.Payment{
		payment("2026-07-01", "-100", "Еда"),
		payment("2026-07-02", "-300", "Еда", "Кафе"),
		payment("2026-07-03", "-50"), // без тега
		payment("2026-07-04", "5000", "Зарплата"),
		payment("2026-06-30", "-999", "Еда"), // вне периода
	}
	rng := dateRange{from: "2026-07-01", to: "2026-07-31"}

	stats, total, base, count := aggregateByTag(payments, rng, false)

	if base != "RUB" {
		t.Errorf("валюта = %q", base)
	}
	if count != 3 {
		t.Errorf("учтено платежей = %d, want 3 (доход и прошлый месяц не считаются)", count)
	}
	if math.Abs(total-450) > 1e-9 {
		t.Errorf("сумма расходов = %v, want 450", total)
	}

	// Отсортировано по убыванию суммы.
	if stats[0].Tag != "Еда" || math.Abs(stats[0].Total-400) > 1e-9 {
		t.Errorf("первым ожидалась «Еда» на 400, получено %+v", stats[0])
	}
	if stats[0].Count != 2 {
		t.Errorf("у «Еды» %d платежей, want 2", stats[0].Count)
	}

	// Платёж с двумя тегами попадает в каждый целиком — сумма долей больше 100%.
	var shareSum float64
	byName := map[string]tagStat{}
	for _, st := range stats {
		shareSum += st.Share
		byName[st.Tag] = st
	}
	if shareSum <= 1.0 {
		t.Errorf("сумма долей = %v, ожидалась больше 1: платёж с двумя тегами учтён в обоих", shareSum)
	}
	if got := byName["Кафе"].Total; math.Abs(got-300) > 1e-9 {
		t.Errorf("«Кафе» = %v, want 300", got)
	}
	if !byName["без тега"].IsOther {
		t.Error("строка «без тега» должна помечаться как служебная")
	}
}

func TestAggregateByTagIncome(t *testing.T) {
	payments := []api.Payment{
		payment("2026-07-04", "5000", "Зарплата"),
		payment("2026-07-05", "-100", "Еда"),
	}
	stats, total, _, count := aggregateByTag(payments, dateRange{}, true)

	if count != 1 || math.Abs(total-5000) > 1e-9 {
		t.Fatalf("доходы: count=%d total=%v, want 1 и 5000", count, total)
	}
	if len(stats) != 1 || stats[0].Tag != "Зарплата" {
		t.Errorf("ожидалась одна строка «Зарплата», получено %+v", stats)
	}
	if math.Abs(stats[0].Share-1) > 1e-9 {
		t.Errorf("доля единственного тега = %v, want 1", stats[0].Share)
	}
}

// Платежи с нулём и с мусором в сумме не должны ни падать, ни попадать в счёт.
func TestAggregateByTagSkipsUnparsable(t *testing.T) {
	payments := []api.Payment{
		payment("2026-07-01", "0", "Еда"),
		payment("2026-07-02", "", "Еда"),
		payment("2026-07-03", "н/д", "Еда"),
		payment("2026-07-04", "-100", "Еда"),
	}
	stats, total, _, count := aggregateByTag(payments, dateRange{}, false)
	if count != 1 || math.Abs(total-100) > 1e-9 {
		t.Fatalf("count=%d total=%v, want 1 и 100", count, total)
	}
	if len(stats) != 1 {
		t.Errorf("ожидалась одна строка, получено %d", len(stats))
	}
}
