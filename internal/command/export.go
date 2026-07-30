package command

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Kaidstor/finances-next/cli/internal/output"
)

const exportHelp = `finances-kai export [фильтры] — zip со счетами за период

Фильтры те же, что у ls; отбор здесь делает сервер.

  finances-kai export --period year
  finances-kai export --from 2026-01-01 --to 2026-06-30 -o 1h2026.zip
  finances-kai export --type expense --tag аренда`

func cmdExport(ctx context.Context, args []string) error {
	fs, profile, _ := newFlagSet("export", exportHelp)
	period := fs.String("period", "month", "today|week|month|year|all")
	from := fs.String("from", "", "начало периода, YYYY-MM-DD")
	to := fs.String("to", "", "конец периода, YYYY-MM-DD")
	kind := fs.String("type", "", "income|expense")
	out := fs.String("o", "", "куда сохранить (по умолчанию имя от сервера); - — в stdout")
	var tags, cps stringList
	fs.Var(&tags, "tag", "тег; флаг можно повторять")
	fs.Var(&cps, "cp", "контрагент; флаг можно повторять")
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	rng, err := resolveRange(*period, *from, *to, time.Now())
	if err != nil {
		return err
	}
	if err := checkEnum("--type", *kind, "income", "expense"); err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}

	tagIDs, cpIDs, err := s.resolveFilters(ctx, tags, cps)
	if err != nil {
		return err
	}

	query := url.Values{}
	if rng.from != "" {
		query.Set("from", rng.from)
	}
	if rng.to != "" {
		query.Set("to", rng.to)
	}
	if *kind != "" {
		query.Set("type", *kind)
	}
	if len(tagIDs) > 0 {
		query.Set("tagIds", strings.Join(tagIDs, ","))
	}
	if len(cpIDs) > 0 {
		query.Set("counterpartyIds", strings.Join(cpIDs, ","))
	}

	zip, suggested, err := s.client.Export(ctx, query)
	if err != nil {
		return err
	}

	if *out == "-" {
		_, err := os.Stdout.Write(zip)
		return err
	}

	name := *out
	if name == "" {
		name = suggested
	}
	if err := os.WriteFile(name, zip, 0o600); err != nil {
		return fmt.Errorf("сохраняю %s: %w", name, err)
	}
	fmt.Printf("%s %s · %s · %s\n", output.Green("сохранено"), name,
		humanSize(len(zip)), output.Dim(rng.String()))
	return nil
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f КБ", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d Б", n)
	}
}
