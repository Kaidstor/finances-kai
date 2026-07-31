package command

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/Kaidstor/finances-next/cli/internal/api"
	"github.com/Kaidstor/finances-next/cli/internal/output"
)

const statsHelp = `finances-kai stats [фильтры] — сводка по тегам за период

  finances-kai stats
  finances-kai stats --period year --income
  finances-kai stats --from 2026-01-01 --to 2026-06-30 --json`

// tagStat — строка сводки. Суммы в базовой валюте: платежи бывают в разных,
// складывать их в исходной нельзя.
type tagStat struct {
	Tag     string  `json:"tag"`
	Total   float64 `json:"total"`
	Count   int     `json:"count"`
	Share   float64 `json:"share"`
	IsOther bool    `json:"-"`
}

func cmdStats(ctx context.Context, args []string) error {
	fs, profile, asJSON := newFlagSet("stats", statsHelp)
	period := fs.String("period", "month", "today|week|month|year|all")
	from := fs.String("from", "", "начало периода, YYYY-MM-DD")
	to := fs.String("to", "", "конец периода, YYYY-MM-DD")
	income := fs.Bool("income", false, "считать доходы вместо расходов")
	limit := fs.Int("limit", 15, "сколько тегов показать; 0 — все")
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	rng, err := resolveRange(*period, *from, *to, time.Now())
	if err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	payments, err := s.client.Payments(ctx)
	if err != nil {
		return err
	}

	stats, total, base, count := aggregateByTag(payments, rng, *income)

	if *asJSON {
		return output.JSON(map[string]any{
			"period":   rng.String(),
			"currency": base,
			"total":    total,
			"count":    count,
			"byTag":    stats,
		})
	}

	label := "Расходы"
	paint := output.Red
	if *income {
		label, paint = "Доходы", output.Green
	}

	if count == 0 {
		fmt.Printf("%s за %s: ничего\n", label, rng)
		return nil
	}

	shown := stats
	if *limit > 0 && len(shown) > *limit {
		shown = shown[:*limit]
	}

	t := output.NewTable("ТЕГ", "СУММА", "ДОЛЯ", "ШТ").RightAlign(1, 2, 3)
	for _, st := range shown {
		name := st.Tag
		if st.IsOther {
			name = output.Dim(name)
		}
		t.Add(
			name,
			output.Amount(strconv.FormatFloat(st.Total, 'f', 2, 64), base),
			fmt.Sprintf("%.0f%%", st.Share*100),
			strconv.Itoa(st.Count),
		)
	}
	t.Render(os.Stdout)

	fmt.Printf("\n%s · %s %s\n", output.Bold(rng.String()), label,
		paint(output.Amount(strconv.FormatFloat(total, 'f', 2, 64), base)))
	if *limit > 0 && len(stats) > *limit {
		fmt.Printf("%s\n", output.Dim(fmt.Sprintf("показано %d тегов из %d", *limit, len(stats))))
	}
	return nil
}

// aggregateByTag раскладывает суммы по тегам. Платёж с несколькими тегами
// попадает в каждый целиком, поэтому сумма долей может превысить 100% —
// доля здесь отвечает на «сколько прошло через тег», а не «какая часть от
// целого», и делить сумму между тегами было бы враньём другого рода.
func aggregateByTag(payments []api.Payment, rng dateRange, income bool) (
	stats []tagStat, total float64, base string, count int,
) {
	base = "RUB"
	byTag := map[string]*tagStat{}
	const untagged = "без тега"

	for _, p := range payments {
		if !rng.contains(p.Date) {
			continue
		}
		v, err := strconv.ParseFloat(p.AmountInBaseCurrency, 64)
		if err != nil || v == 0 {
			continue
		}
		if income != (v > 0) {
			continue
		}
		if p.BaseCurrency != "" {
			base = p.BaseCurrency
		}

		amount := v
		if !income {
			amount = -v
		}
		total += amount
		count++

		names := make([]string, 0, len(p.Tags))
		for _, tag := range p.Tags {
			names = append(names, tag.Name)
		}
		if len(names) == 0 {
			names = []string{untagged}
		}
		for _, name := range names {
			st, ok := byTag[name]
			if !ok {
				st = &tagStat{Tag: name, IsOther: name == untagged}
				byTag[name] = st
			}
			st.Total += amount
			st.Count++
		}
	}

	stats = make([]tagStat, 0, len(byTag))
	for _, st := range byTag {
		if total != 0 {
			st.Share = st.Total / total
		}
		stats = append(stats, *st)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Total != stats[j].Total {
			return stats[i].Total > stats[j].Total
		}
		return stats[i].Tag < stats[j].Tag
	})
	return stats, total, base, count
}

const forecastHelp = `finances-kai forecast [--months N] — прогноз трат по подпискам

  finances-kai forecast
  finances-kai forecast --months 3
  finances-kai forecast --by-subscription`

func cmdForecast(ctx context.Context, args []string) error {
	fs, profile, asJSON := newFlagSet("forecast", forecastHelp)
	months := fs.Int("months", 6, "горизонт в месяцах, 1..60")
	bySub := fs.Bool("by-subscription", false, "разбивка по подпискам вместо месяцев")
	if err := fs.Parse(args); err != nil {
		return errParsed
	}
	if *months < 1 || *months > 60 {
		return fmt.Errorf("--months: ожидается 1..60, получено %d", *months)
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	f, err := s.client.Forecast(ctx, *months)
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(f)
	}

	amount := func(v float64) string {
		return output.Amount(strconv.FormatFloat(v, 'f', 2, 64), f.BaseCurrency)
	}

	if *bySub {
		if len(f.BySubscription) == 0 {
			fmt.Println("Активных подписок нет")
			return nil
		}
		t := output.NewTable("ПОДПИСКА", "ЗА ПЕРИОД", "ПЛАТЕЖЕЙ").RightAlign(1, 2)
		for _, sub := range f.BySubscription {
			t.Add(sub.SubscriptionName, amount(sub.Total), strconv.Itoa(sub.Count))
		}
		t.Render(os.Stdout)
	} else {
		if len(f.ByMonth) == 0 {
			fmt.Println("Платежей по подпискам в горизонте нет")
			return nil
		}
		t := output.NewTable("МЕСЯЦ", "СУММА", "ПЛАТЕЖЕЙ").RightAlign(1, 2)
		for _, m := range f.ByMonth {
			t.Add(m.MonthName, amount(m.Total), strconv.Itoa(len(m.Payments)))
		}
		t.Render(os.Stdout)
	}

	fmt.Printf("\n%s\n", output.Bold("Итого по подпискам"))
	summary := output.NewTable("", "").RightAlign(1)
	summary.Add("месяц", amount(f.Summary.Total1Month))
	summary.Add("3 мес", amount(f.Summary.Total3Months))
	summary.Add("6 мес", amount(f.Summary.Total6Months))
	summary.Add("год", amount(f.Summary.TotalYear))
	summary.Render(os.Stdout)

	fmt.Printf("%s\n", output.Dim(fmt.Sprintf(
		"горизонт %d %s", *months, plural(*months, "месяц", "месяца", "месяцев"))))
	return nil
}
