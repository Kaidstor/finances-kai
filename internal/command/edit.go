package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Kaidstor/finances-kai/internal/api"
	"github.com/Kaidstor/finances-kai/internal/output"
)

const editHelp = `finances-kai edit <id> [что менять] — правка платежа

Меняются только переданные поля, остальное остаётся как было.

  finances-kai edit 11111111 --amount -300
  finances-kai edit 11111111 --paid --description "кофе и круассан"
  finances-kai edit 11111111 --tag еда --tag кафе      # теги заменяются целиком
  finances-kai edit 11111111 --description ""          # стереть описание`

func cmdEdit(ctx context.Context, pr *output.Printer, args []string) error {
	fs, profile := newFlagSet(pr, "edit", editHelp)
	amount := fs.String("amount", "", "новая сумма; минус — расход")
	date := fs.String("date", "", "today|yesterday|tomorrow|YYYY-MM-DD")
	currency := fs.String("currency", "", "RUB|USD|KZT")
	description := fs.String("description", "", "описание; пустая строка стирает")
	paid := fs.Bool("paid", false, "пометить оплаченным")
	unpaid := fs.Bool("unpaid", false, "пометить неоплаченным")
	var tags, cps stringList
	fs.Var(&tags, "tag", "заменить теги; флаг можно повторять")
	fs.Var(&cps, "cp", "заменить контрагентов; флаг можно повторять")

	id, err := parseWithID(fs, args)
	if err != nil {
		return err
	}
	if *paid && *unpaid {
		return usageErr("--paid и --unpaid взаимоисключающие")
	}
	if err := checkEnum("--currency", *currency, "RUB", "USD", "KZT"); err != nil {
		return err
	}

	var req api.UpdatePayment
	changed := false

	if isSet(fs, "amount") {
		if _, err := strconv.ParseFloat(*amount, 64); err != nil {
			return usageErr("--amount: %q не число", *amount)
		}
		req.Amount, changed = amount, true
	}
	if isSet(fs, "date") {
		day, err := resolveDay(*date, time.Now())
		if err != nil {
			return err
		}
		req.Date, changed = &day, true
	}
	if isSet(fs, "currency") {
		req.Currency, changed = currency, true
	}
	if isSet(fs, "description") {
		req.Description, changed = description, true
	}
	if *paid {
		req.Status, changed = strPtr("paid"), true
	}
	if *unpaid {
		req.Status, changed = strPtr("unpaid"), true
	}

	if !changed && len(tags) == 0 && len(cps) == 0 {
		return usageErr("нечего менять: не передано ни одного поля")
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	before, err := s.findPayment(ctx, id)
	if err != nil {
		return err
	}

	if len(tags) > 0 || len(cps) > 0 {
		tagIDs, cpIDs, err := s.resolveFilters(ctx, tags, cps)
		if err != nil {
			return err
		}
		if len(tags) > 0 {
			req.TagIDs = &tagIDs
		}
		if len(cps) > 0 {
			req.CounterpartyIDs = &cpIDs
		}
	}

	after, err := s.client.UpdatePayment(ctx, before.ID, req)
	if err != nil {
		return err
	}

	if pr.JSON {
		return pr.Data(after)
	}
	fmt.Println(output.Green("обновлён"), after.ID)
	printDiff(before, after)
	return nil
}

// printDiff показывает, что именно изменилось: при правке вслепую это
// единственный способ заметить, что поле уехало не туда, куда ждали.
func printDiff(before, after api.Payment) {
	type field struct{ name, old, new string }
	fields := []field{
		{"сумма", output.Amount(before.Amount, before.Currency), output.Amount(after.Amount, after.Currency)},
		{"дата", output.Date(before.Date), output.Date(after.Date)},
		{"статус", statusLabel(before.Status), statusLabel(after.Status)},
		{"описание", deref(before.Description), deref(after.Description)},
		{"теги", joinTags(before.Tags), joinTags(after.Tags)},
		{"контрагенты", joinCounterparties(before.Counterparties), joinCounterparties(after.Counterparties)},
	}

	for _, f := range fields {
		if f.old == f.new {
			continue
		}
		fmt.Printf("  %-12s %s → %s\n", f.name, output.Dim(orDash(f.old)), orDash(f.new))
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
