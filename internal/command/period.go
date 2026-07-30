package command

import (
	"fmt"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// dateRange — полуоткрытый период в локальных календарных датах. Пустая
// граница означает «без ограничения с этой стороны».
type dateRange struct {
	from, to string
}

func (r dateRange) empty() bool { return r.from == "" && r.to == "" }

// resolveRange превращает --period/--from/--to в конкретные даты. Явные
// границы сильнее периода: указавший --from не ждёт, что месяц его обрежет.
func resolveRange(period, from, to string, now time.Time) (dateRange, error) {
	if from != "" || to != "" {
		if err := checkDate(from); err != nil {
			return dateRange{}, fmt.Errorf("--from: %w", err)
		}
		if err := checkDate(to); err != nil {
			return dateRange{}, fmt.Errorf("--to: %w", err)
		}
		if from != "" && to != "" && from > to {
			return dateRange{}, fmt.Errorf("--from %s позже --to %s", from, to)
		}
		return dateRange{from: from, to: to}, nil
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch strings.ToLower(period) {
	case "", "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return dateRange{start.Format(dateLayout), start.AddDate(0, 1, -1).Format(dateLayout)}, nil
	case "today":
		return dateRange{today.Format(dateLayout), today.Format(dateLayout)}, nil
	case "week":
		// В Go воскресенье — нулевой день недели, а неделя у нас с понедельника.
		offset := (int(today.Weekday()) + 6) % 7
		start := today.AddDate(0, 0, -offset)
		return dateRange{start.Format(dateLayout), start.AddDate(0, 0, 6).Format(dateLayout)}, nil
	case "year":
		start := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
		return dateRange{start.Format(dateLayout), start.AddDate(1, 0, -1).Format(dateLayout)}, nil
	case "all":
		return dateRange{}, nil
	default:
		return dateRange{}, fmt.Errorf("неизвестный период %q: today, week, month, year или all", period)
	}
}

func checkDate(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse(dateLayout, s); err != nil {
		return fmt.Errorf("ожидается YYYY-MM-DD, получено %q", s)
	}
	return nil
}

// resolveDay разбирает --date: today, yesterday, tomorrow или YYYY-MM-DD.
// Дата берётся локальная: toISOString() сдвинул бы её на день назад.
func resolveDay(s string, now time.Time) (string, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "today", "сегодня":
		return today.Format(dateLayout), nil
	case "yesterday", "вчера":
		return today.AddDate(0, 0, -1).Format(dateLayout), nil
	case "tomorrow", "завтра":
		return today.AddDate(0, 0, 1).Format(dateLayout), nil
	}
	if err := checkDate(s); err != nil {
		return "", fmt.Errorf("--date: %w", err)
	}
	return s, nil
}

// inRange проверяет дату платежа (timestamp-строка от сервера) на попадание в
// период. Сравнение строковое — для YYYY-MM-DD это то же, что хронологическое.
func (r dateRange) contains(paymentDate string) bool {
	day := paymentDate
	if i := strings.IndexAny(day, "T "); i > 0 {
		day = day[:i]
	}
	if r.from != "" && day < r.from {
		return false
	}
	if r.to != "" && day > r.to {
		return false
	}
	return true
}

func (r dateRange) String() string {
	switch {
	case r.empty():
		return "всё время"
	case r.from == r.to:
		return r.from
	case r.from == "":
		return "по " + r.to
	case r.to == "":
		return "с " + r.from
	default:
		return r.from + " — " + r.to
	}
}
