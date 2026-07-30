package command

import (
	"testing"
	"time"
)

// Среда, 15 июля 2026, полдень — удобно тем, что не край месяца и не воскресенье.
var ref = time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)

func TestResolveRange(t *testing.T) {
	cases := []struct {
		name             string
		period, from, to string
		wantFrom, wantTo string
	}{
		{name: "по умолчанию месяц", wantFrom: "2026-07-01", wantTo: "2026-07-31"},
		{name: "month", period: "month", wantFrom: "2026-07-01", wantTo: "2026-07-31"},
		{name: "today", period: "today", wantFrom: "2026-07-15", wantTo: "2026-07-15"},
		{name: "week с понедельника", period: "week", wantFrom: "2026-07-13", wantTo: "2026-07-19"},
		{name: "year", period: "year", wantFrom: "2026-01-01", wantTo: "2026-12-31"},
		{name: "all без границ", period: "all"},
		{name: "явные границы сильнее периода", period: "month",
			from: "2026-01-01", to: "2026-03-31", wantFrom: "2026-01-01", wantTo: "2026-03-31"},
		{name: "только from", from: "2026-05-01", wantFrom: "2026-05-01"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveRange(c.period, c.from, c.to, ref)
			if err != nil {
				t.Fatalf("resolveRange: %v", err)
			}
			if got.from != c.wantFrom || got.to != c.wantTo {
				t.Errorf("got (%q, %q), want (%q, %q)", got.from, got.to, c.wantFrom, c.wantTo)
			}
		})
	}
}

func TestResolveRangeErrors(t *testing.T) {
	cases := []struct{ name, period, from, to string }{
		{name: "неизвестный период", period: "decade"},
		{name: "мусор в from", from: "15.07.2026"},
		{name: "from позже to", from: "2026-07-10", to: "2026-07-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := resolveRange(c.period, c.from, c.to, ref); err == nil {
				t.Error("ожидалась ошибка, получено nil")
			}
		})
	}
}

// Неделя, начинающаяся в воскресенье, — та ловушка, на которой ломается наивный
// int(Weekday()): в Go воскресенье это 0, а не 7.
func TestResolveRangeWeekOnSunday(t *testing.T) {
	sunday := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	got, err := resolveRange("week", "", "", sunday)
	if err != nil {
		t.Fatalf("resolveRange: %v", err)
	}
	if got.from != "2026-07-13" || got.to != "2026-07-19" {
		t.Errorf("got (%q, %q), want (2026-07-13, 2026-07-19)", got.from, got.to)
	}
}

func TestResolveDay(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "2026-07-15"},
		{"today", "2026-07-15"},
		{"сегодня", "2026-07-15"},
		{"yesterday", "2026-07-14"},
		{"вчера", "2026-07-14"},
		{"tomorrow", "2026-07-16"},
		{"2026-01-02", "2026-01-02"},
	}
	for _, c := range cases {
		got, err := resolveDay(c.in, ref)
		if err != nil {
			t.Fatalf("resolveDay(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("resolveDay(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := resolveDay("позавчера", ref); err == nil {
		t.Error("ожидалась ошибка на неизвестное слово")
	}
}

func TestRangeContains(t *testing.T) {
	r := dateRange{from: "2026-07-01", to: "2026-07-31"}
	cases := []struct {
		date string
		want bool
	}{
		{"2026-07-15T00:00:00", true},
		{"2026-07-15 10:30:00", true},
		{"2026-07-01", true},
		{"2026-07-31T23:59:59", true},
		{"2026-06-30", false},
		{"2026-08-01", false},
	}
	for _, c := range cases {
		if got := r.contains(c.date); got != c.want {
			t.Errorf("contains(%q) = %v, want %v", c.date, got, c.want)
		}
	}

	open := dateRange{}
	if !open.contains("1999-01-01") {
		t.Error("пустой период должен принимать любую дату")
	}
}
