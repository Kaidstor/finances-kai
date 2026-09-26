package command

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Kaidstor/finances-kai/internal/api"
	"github.com/Kaidstor/finances-kai/internal/output"
)

const lsHelp = `finances-kai ls [фильтры] — платежи за период (по умолчанию текущий месяц)

  finances-kai ls
  finances-kai ls --period year --type expense
  finances-kai ls --from 2026-01-01 --to 2026-03-31 --tag еда`

func cmdList(ctx context.Context, pr *output.Printer, args []string) error {
	fs, profile := newFlagSet(pr, "ls", lsHelp)
	period := fs.String("period", "month", "today|week|month|year|all")
	from := fs.String("from", "", "начало периода, YYYY-MM-DD")
	to := fs.String("to", "", "конец периода, YYYY-MM-DD")
	kind := fs.String("type", "", "income|expense")
	status := fs.String("status", "", "paid|unpaid")
	search := fs.String("search", "", "подстрока в описании")
	limit := fs.Int("limit", 50, "сколько строк показать; 0 — все")
	var tags, cps stringList
	fs.Var(&tags, "tag", "тег; флаг можно повторять")
	fs.Var(&cps, "cp", "контрагент; флаг можно повторять")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	rng, err := resolveRange(*period, *from, *to, time.Now())
	if err != nil {
		return err
	}
	if err := checkEnum("--type", *kind, "income", "expense"); err != nil {
		return err
	}
	if err := checkEnum("--status", *status, "paid", "unpaid"); err != nil {
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

	// Сервер отдаёт все платежи разом и фильтров в GET /api/payments нет,
	// поэтому отбор идёт здесь.
	tagIDs, cpIDs, err := s.resolveFilters(ctx, tags, cps)
	if err != nil {
		return err
	}

	filtered := payments[:0:0]
	for _, p := range payments {
		if !rng.contains(p.Date) ||
			!matchesKind(p, *kind) ||
			(*status != "" && p.Status != *status) ||
			!hasAnyTag(p, tagIDs) ||
			!hasAnyCounterparty(p, cpIDs) ||
			!matchesSearch(p, *search) {
			continue
		}
		filtered = append(filtered, p)
	}

	// Порядок задаём сами: полагаться на сортировку сервера — значит менять
	// вывод CLI вместе с любой правкой orderBy в роуте.
	sort.SliceStable(filtered, func(i, j int) bool {
		return output.Date(filtered[i].Date) > output.Date(filtered[j].Date)
	})

	if pr.JSON {
		return pr.Data(filtered)
	}
	printPayments(filtered, rng, *limit)
	return nil
}

func (s *session) resolveFilters(ctx context.Context, tags, cps []string) (tagIDs, cpIDs []string, err error) {
	if len(tags) > 0 {
		idx, err := s.loadTags(ctx)
		if err != nil {
			return nil, nil, err
		}
		if tagIDs, err = idx.resolveAll(tags); err != nil {
			return nil, nil, err
		}
	}
	if len(cps) > 0 {
		idx, err := s.loadCounterparties(ctx)
		if err != nil {
			return nil, nil, err
		}
		if cpIDs, err = idx.resolveAll(cps); err != nil {
			return nil, nil, err
		}
	}
	return tagIDs, cpIDs, nil
}

func printPayments(payments []api.Payment, rng dateRange, limit int) {
	if len(payments) == 0 {
		fmt.Printf("Платежей за %s нет\n", rng)
		return
	}

	shown := payments
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}

	t := output.NewTable("ДАТА", "СУММА", "ОПИСАНИЕ", "ТЕГИ", "КОНТРАГЕНТЫ", "ID").RightAlign(1)
	for _, p := range shown {
		t.Add(
			output.Date(p.Date),
			amountCell(p),
			describe(p),
			joinTags(p.Tags),
			joinCounterparties(p.Counterparties),
			output.Dim(shortID(p.ID)),
		)
	}
	t.Render(os.Stdout)

	income, expense, base := totals(payments)
	fmt.Printf("\n%s · %d %s",
		output.Bold(rng.String()), len(payments), plural(len(payments), "запись", "записи", "записей"))
	if limit > 0 && len(payments) > limit {
		fmt.Printf(" (показано %d)", limit)
	}
	fmt.Printf("\nдоход %s · расход %s · сальдо %s\n",
		output.Green(output.Amount(fmt.Sprintf("%.2f", income), base)),
		output.Red(output.Amount(fmt.Sprintf("%.2f", expense), base)),
		output.Amount(fmt.Sprintf("%.2f", income-expense), base))
}

// totals считает в базовой валюте пользователя: платежи бывают в разных
// валютах, складывать их «как есть» нельзя.
func totals(payments []api.Payment) (income, expense float64, base string) {
	base = "RUB"
	for _, p := range payments {
		if p.BaseCurrency != "" {
			base = p.BaseCurrency
		}
		v, err := strconv.ParseFloat(p.AmountInBaseCurrency, 64)
		if err != nil {
			continue
		}
		if v >= 0 {
			income += v
		} else {
			expense += -v
		}
	}
	return income, expense, base
}

const addHelp = `finances-kai add <сумма> [описание] — создать платёж

Сумма с минусом — расход, без минуса — доход.

  finances-kai add -250 "кофе" --tag еда --paid
  finances-kai add 120000 "зарплата" --date yesterday --paid
  finances-kai add -1500 --tag подписки --cp Netflix --currency USD`

func cmdAdd(ctx context.Context, pr *output.Printer, args []string) error {
	// Сумма идёт первым позиционным аргументом, но "-250" неотличимо от флага,
	// поэтому забираем её до разбора флагов.
	amount, rest, err := takeAmount(args)
	if err != nil {
		return err
	}

	fs, profile := newFlagSet(pr, "add", addHelp)
	date := fs.String("date", "today", "today|yesterday|tomorrow|YYYY-MM-DD")
	currency := fs.String("currency", "", "RUB|USD|KZT (по умолчанию валюта профиля)")
	paid := fs.Bool("paid", false, "пометить оплаченным (по умолчанию неоплаченный, как в веб-форме)")
	var tags, cps stringList
	fs.Var(&tags, "tag", "тег; флаг можно повторять")
	fs.Var(&cps, "cp", "контрагент; флаг можно повторять")

	words, flags := splitLeading(rest)
	if err := parseFlags(fs, flags); err != nil {
		return err
	}

	// Слова после флагов тоже относятся к описанию: `add -250 --paid кофе`.
	description := strings.TrimSpace(strings.Join(append(words, fs.Args()...), " "))
	day, err := resolveDay(*date, time.Now())
	if err != nil {
		return err
	}
	if err := checkEnum("--currency", *currency, "RUB", "USD", "KZT"); err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	if *currency == "" {
		*currency = s.defaultCurrency(ctx)
	}

	tagIDs, cpIDs, err := s.resolveFilters(ctx, tags, cps)
	if err != nil {
		return err
	}

	req := api.CreatePayment{
		Status:          statusOf(*paid),
		Amount:          amount,
		Currency:        *currency,
		Date:            day,
		TagIDs:          tagIDs,
		CounterpartyIDs: cpIDs,
	}
	if description != "" {
		req.Description = &description
	}

	created, err := s.client.CreatePayment(ctx, req)
	if err != nil {
		return err
	}

	if pr.JSON {
		return pr.Data(created)
	}
	fmt.Printf("%s %s · %s\n", output.Green("создан"), colorAmount(created), describe(created))
	fmt.Printf("%s\n", output.Dim(created.ID))
	return nil
}

const newHelp = `finances-kai new <шаблон> [сумма] — создать платёж по шаблону

Знак суммы берётся из типа шаблона: expense — расход, income — доход.
Если сумма не указана, берётся из шаблона.

  finances-kai new обед
  finances-kai new "аренда" 45000 --date yesterday
  finances-kai new обед --description "бизнес-ланч у офиса"`

func cmdNew(ctx context.Context, pr *output.Printer, args []string) error {
	fs, profile := newFlagSet(pr, "new", newHelp)
	date := fs.String("date", "today", "today|yesterday|tomorrow|YYYY-MM-DD")
	paid := fs.Bool("paid", false, "пометить оплаченным (иначе статус берётся из шаблона)")
	description := fs.String("description", "", "описание вместо взятого из шаблона; пустая строка — без описания")

	words, flags := splitLeading(args)
	if err := parseFlags(fs, flags); err != nil {
		return err
	}
	words = append(words, fs.Args()...)
	if len(words) == 0 {
		return errUsage(fs, "не задано имя шаблона")
	}
	if len(words) > 2 {
		return usageErr("ожидались имя шаблона и, опционально, сумма; получено %d аргументов", len(words))
	}

	name := words[0]
	override := ""
	if len(words) > 1 {
		override = words[1]
		if _, err := strconv.ParseFloat(override, 64); err != nil {
			return usageErr("сумма %q не число", override)
		}
	}

	day, err := resolveDay(*date, time.Now())
	if err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}

	templates, err := s.client.Templates(ctx)
	if err != nil {
		return err
	}
	tpl, err := pickTemplate(templates, name)
	if err != nil {
		return err
	}

	amount := override
	if amount == "" {
		amount = strings.TrimSpace(deref(tpl.Amount))
	}
	if amount == "" {
		return usageErr("в шаблоне %q нет суммы — укажите её вторым аргументом", tpl.Name)
	}
	// Шаблон хранит сумму без знака, а направление — в поле type.
	if deref(tpl.Type) == "expense" && !strings.HasPrefix(amount, "-") {
		amount = "-" + amount
	}

	status := deref(tpl.Status)
	if *paid {
		status = "paid"
	}
	if status == "" {
		status = "unpaid"
	}

	currency := deref(tpl.Currency)
	if currency == "" {
		currency = s.defaultCurrency(ctx)
	}

	req := api.CreatePayment{
		Status:          status,
		Amount:          amount,
		Currency:        currency,
		Date:            day,
		TagIDs:          tpl.TagIDs,
		CounterpartyIDs: tpl.CounterpartyIDs,
	}
	switch {
	case isSet(fs, "description"):
		req.Description = description
	case strings.TrimSpace(deref(tpl.Description)) != "":
		d := strings.TrimSpace(deref(tpl.Description))
		req.Description = &d
	default:
		req.Description = &tpl.Name
	}

	created, err := s.client.CreatePayment(ctx, req)
	if err != nil {
		return err
	}

	if pr.JSON {
		return pr.Data(created)
	}
	fmt.Printf("%s %s · %s %s\n", output.Green("создан"), colorAmount(created),
		describe(created), output.Dim("(шаблон "+tpl.Name+")"))
	fmt.Printf("%s\n", output.Dim(created.ID))
	return nil
}

func pickTemplate(templates []api.Template, query string) (api.Template, error) {
	names := make([]string, 0, len(templates))
	for _, t := range templates {
		names = append(names, t.Name)
	}
	matches := match(query, names)

	switch len(matches) {
	case 1:
		return templates[matches[0]], nil
	case 0:
		return api.Template{}, notFound("шаблон %q не найден; есть: %s", query, preview(names))
	default:
		var found []string
		for _, m := range matches {
			found = append(found, templates[m].Name)
		}
		return api.Template{}, ambiguous("шаблон %q подходит к нескольким: %s — уточните",
			query, strings.Join(found, ", "))
	}
}

const showHelp = `finances-kai show <id> — карточка платежа

id можно указывать префиксом — тем, что печатает ls.`

func cmdShow(ctx context.Context, pr *output.Printer, args []string) error {
	fs, profile := newFlagSet(pr, "show", showHelp)
	id, err := parseWithID(fs, args)
	if err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	p, err := s.findPayment(ctx, id)
	if err != nil {
		return err
	}

	if pr.JSON {
		return pr.Data(p)
	}

	fmt.Println(output.Bold(colorAmount(p)), "·", describe(p))
	fmt.Println(output.Dim("дата       "), output.Date(p.Date))
	fmt.Println(output.Dim("статус     "), statusLabel(p.Status))
	if p.Currency != p.BaseCurrency {
		fmt.Println(output.Dim("в базовой  "), output.Amount(p.AmountInBaseCurrency, p.BaseCurrency))
	}
	if len(p.Tags) > 0 {
		fmt.Println(output.Dim("теги       "), joinTags(p.Tags))
	}
	if len(p.Counterparties) > 0 {
		fmt.Println(output.Dim("контрагенты"), joinCounterparties(p.Counterparties))
	}
	fmt.Println(output.Dim("id         "), p.ID)
	if d := strings.TrimSpace(deref(p.Details)); d != "" {
		fmt.Println("\n" + d)
	}
	return nil
}

const rmHelp = `finances-kai rm <id> — удалить платёж

id можно указывать префиксом — тем, что печатает ls.`

func cmdRemove(ctx context.Context, pr *output.Printer, args []string) error {
	fs, profile := newFlagSet(pr, "rm", rmHelp)
	yes := fs.Bool("yes", false, "не спрашивать подтверждения")
	id, err := parseWithID(fs, args)
	if err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	p, err := s.findPayment(ctx, id)
	if err != nil {
		return err
	}

	if !*yes {
		fmt.Fprintf(pr.Info(), "Удалить %s · %s от %s?\n", colorAmount(p), describe(p), output.Date(p.Date))
		ok, err := confirm(pr.Info(), "Введите y для подтверждения: ")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(pr.Info(), "отменено")
			return pr.Data(deleted{ID: p.ID, Deleted: false})
		}
	}

	if err := s.client.DeletePayment(ctx, p.ID); err != nil {
		return err
	}
	if pr.JSON {
		return pr.Data(deleted{ID: p.ID, Deleted: true})
	}
	fmt.Println(output.Yellow("удалён"), p.ID)
	return nil
}

// deleted — data конверта у команд удаления. Deleted=false — человек ответил
// «нет» на вопрос подтверждения.
type deleted struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Deleted bool   `json:"deleted"`
}

// findPayment принимает и полный UUID, и префикс из вывода ls.
func (s *session) findPayment(ctx context.Context, query string) (api.Payment, error) {
	if len(query) == 36 && strings.Count(query, "-") == 4 {
		return s.client.Payment(ctx, query)
	}

	payments, err := s.client.Payments(ctx)
	if err != nil {
		return api.Payment{}, err
	}
	var found []api.Payment
	for _, p := range payments {
		if strings.HasPrefix(p.ID, query) {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return api.Payment{}, notFound("платёж %q не найден", query)
	default:
		return api.Payment{}, ambiguous("префикс %q подходит к %d платежам — укажите больше символов",
			query, len(found))
	}
}
