package command

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Kaidstor/finances-next/cli/internal/config"
	"github.com/Kaidstor/finances-next/cli/internal/output"
)

const exportHelp = `finances-kai export [набор] [фильтры] — zip со счетами за период

Фильтры те же, что у ls; отбор здесь делает сервер.

  finances-kai export --period year
  finances-kai export --from 2026-01-01 --to 2026-06-30 -o 1h2026.zip
  finances-kai export --type expense --tag аренда

Набор фильтров можно сохранить и переиспользовать, задавая период при запуске:

  finances-kai export --save аренда --type expense --tag аренда --cp Арендодатель
  finances-kai export аренда --period year
  finances-kai export аренда --from 2026-01-01 --to 2026-06-30
  finances-kai export --list
  finances-kai export --forget аренда`

func cmdExport(ctx context.Context, args []string) error {
	fs, profile, asJSON := newFlagSet("export", exportHelp)
	period := fs.String("period", "month", "today|week|month|year|all")
	from := fs.String("from", "", "начало периода, YYYY-MM-DD")
	to := fs.String("to", "", "конец периода, YYYY-MM-DD")
	kind := fs.String("type", "", "income|expense")
	out := fs.String("o", "", "куда сохранить (по умолчанию имя от сервера); - — в stdout")
	save := fs.String("save", "", "сохранить фильтры набором под этим именем")
	forget := fs.String("forget", "", "удалить сохранённый набор")
	list := fs.Bool("list", false, "показать сохранённые наборы")
	var tags, cps stringList
	fs.Var(&tags, "tag", "тег; флаг можно повторять")
	fs.Var(&cps, "cp", "контрагент; флаг можно повторять")

	words, flags := splitLeading(args)
	if err := fs.Parse(flags); err != nil {
		return errParsed
	}
	words = append(words, fs.Args()...)
	if len(words) > 1 {
		return fmt.Errorf("ожидалось одно имя набора, получено %d аргументов", len(words))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch {
	case *list:
		return listExportPresets(cfg, *asJSON)
	case *forget != "":
		return forgetExportPreset(cfg, *forget)
	}

	preset := config.ExportPreset{
		Period: *period, From: *from, To: *to, Type: *kind,
		Tags: tags, Counterparties: cps,
	}

	// Имя набора первым аргументом: сохранённые значения берутся за основу,
	// а флаги этого запуска их перекрывают — так и задаётся период.
	if len(words) == 1 {
		saved, ok := cfg.Exports[words[0]]
		if !ok {
			return fmt.Errorf("набор %q не найден; есть: %s",
				words[0], preview(cfg.ExportNames()))
		}
		preset = mergePreset(saved, fs, tags, cps)
	}

	if *save != "" {
		cfg.SetExport(*save, preset)
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("%s набор %s: %s\n", output.Green("сохранён"), output.Bold(*save),
			describePreset(preset))
		fmt.Printf("выгрузить: finances-kai export %s --period month\n", *save)
		return nil
	}

	rng, err := resolveRange(preset.Period, preset.From, preset.To, time.Now())
	if err != nil {
		return err
	}
	if err := checkEnum("--type", preset.Type, "income", "expense"); err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	tagIDs, cpIDs, err := s.resolveFilters(ctx, preset.Tags, preset.Counterparties)
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
	if preset.Type != "" {
		query.Set("type", preset.Type)
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

// mergePreset накладывает флаги запуска на сохранённый набор. Учитывается
// именно факт передачи флага: у --period есть значение по умолчанию, и без
// проверки оно затирало бы сохранённые границы при каждом запуске.
func mergePreset(saved config.ExportPreset, fs *flag.FlagSet, tags, cps []string) config.ExportPreset {
	p := saved

	explicitPeriod := isSet(fs, "period")
	explicitRange := isSet(fs, "from") || isSet(fs, "to")

	if explicitPeriod {
		p.Period = fs.Lookup("period").Value.String()
		// Период и явные границы взаимоисключающи: сохранённые from/to молча
		// пересилили бы переданный период, потому что они приоритетнее.
		if !explicitRange {
			p.From, p.To = "", ""
		}
	}
	if isSet(fs, "from") {
		p.From = fs.Lookup("from").Value.String()
	}
	if isSet(fs, "to") {
		p.To = fs.Lookup("to").Value.String()
	}
	if isSet(fs, "type") {
		p.Type = fs.Lookup("type").Value.String()
	}
	if len(tags) > 0 {
		p.Tags = tags
	}
	if len(cps) > 0 {
		p.Counterparties = cps
	}

	// Набор без периода и без границ означает «период задаётся при запуске»;
	// без этого resolveRange получил бы пустую строку и вернул текущий месяц,
	// что как раз и ожидается по умолчанию.
	if p.Period == "" && p.From == "" && p.To == "" {
		p.Period = "month"
	}
	return p
}

func listExportPresets(cfg *config.Config, asJSON bool) error {
	if asJSON {
		return output.JSON(cfg.Exports)
	}
	if len(cfg.Exports) == 0 {
		fmt.Println("Сохранённых наборов нет. Завести: finances-kai export --save <имя> [фильтры]")
		return nil
	}
	t := output.NewTable("НАБОР", "ФИЛЬТРЫ")
	for _, name := range cfg.ExportNames() {
		t.Add(name, describePreset(cfg.Exports[name]))
	}
	t.Render(os.Stdout)
	return nil
}

func forgetExportPreset(cfg *config.Config, name string) error {
	if _, ok := cfg.Exports[name]; !ok {
		return fmt.Errorf("набор %q не найден; есть: %s", name, preview(cfg.ExportNames()))
	}
	delete(cfg.Exports, name)
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("%s набор %s\n", output.Yellow("удалён"), name)
	return nil
}

func describePreset(p config.ExportPreset) string {
	var parts []string
	switch {
	case p.From != "" || p.To != "":
		parts = append(parts, dateRange{from: p.From, to: p.To}.String())
	case p.Period != "" && p.Period != "month":
		parts = append(parts, "период "+p.Period)
	}
	if p.Type == "income" {
		parts = append(parts, "доходы")
	}
	if p.Type == "expense" {
		parts = append(parts, "расходы")
	}
	if len(p.Tags) > 0 {
		parts = append(parts, "теги: "+strings.Join(p.Tags, ", "))
	}
	if len(p.Counterparties) > 0 {
		parts = append(parts, "контрагенты: "+strings.Join(p.Counterparties, ", "))
	}
	if len(parts) == 0 {
		return output.Dim("без фильтров")
	}
	return strings.Join(parts, " · ")
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
