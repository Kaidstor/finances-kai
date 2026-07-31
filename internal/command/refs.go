package command

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Kaidstor/finances-next/cli/internal/api"
	"github.com/Kaidstor/finances-next/cli/internal/output"
)

const tagsHelp = `finances-kai tags [add <имя>] — теги

  finances-kai tags
  finances-kai tags add "подписки" --color '#3b82f6'`

func cmdTags(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "add" {
		return cmdTagAdd(ctx, args[1:])
	}

	fs, profile, asJSON := newFlagSet("tags", tagsHelp)
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	idx, err := s.loadTags(ctx)
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(idx.tags)
	}
	if len(idx.tags) == 0 {
		fmt.Println("Тегов пока нет")
		return nil
	}

	t := output.NewTable("ТЕГ", "ЦВЕТ", "ID")
	for _, tag := range idx.tags {
		t.Add(idx.path(tag), deref(tag.Color), output.Dim(shortID(tag.ID)))
	}
	t.Render(os.Stdout)
	return nil
}

func cmdTagAdd(ctx context.Context, args []string) error {
	fs, profile, asJSON := newFlagSet("tags add", tagsHelp)
	color := fs.String("color", "", "hex-цвет, например #3b82f6")
	name, err := parseWithName(fs, args)
	if err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}

	req := api.CreateTag{Name: name}
	if *color != "" {
		req.Color = color
	}
	tag, err := s.client.CreateTag(ctx, req)
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(tag)
	}
	fmt.Printf("%s тег %s %s\n", output.Green("создан"), tag.Name, output.Dim(tag.ID))
	return nil
}

const cpHelp = `finances-kai cp [add <имя>] — контрагенты

  finances-kai cp
  finances-kai cp add "Азбука вкуса" --type organization --icon 🛒`

func cmdCounterparties(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "add" {
		return cmdCounterpartyAdd(ctx, args[1:])
	}

	fs, profile, asJSON := newFlagSet("cp", cpHelp)
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	idx, err := s.loadCounterparties(ctx)
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(idx.items)
	}
	if len(idx.items) == 0 {
		fmt.Println("Контрагентов пока нет")
		return nil
	}

	t := output.NewTable("КОНТРАГЕНТ", "ТИП", "ID")
	for _, c := range idx.items {
		name := c.Name
		if icon := strings.TrimSpace(deref(c.Icon)); icon != "" {
			name = icon + " " + name
		}
		t.Add(name, typeLabel(c.Type), output.Dim(shortID(c.ID)))
	}
	t.Render(os.Stdout)
	return nil
}

func cmdCounterpartyAdd(ctx context.Context, args []string) error {
	fs, profile, asJSON := newFlagSet("cp add", cpHelp)
	kind := fs.String("type", "organization", "organization|person")
	icon := fs.String("icon", "", "эмодзи")
	name, err := parseWithName(fs, args)
	if err != nil {
		return err
	}
	if err := checkEnum("--type", *kind, "organization", "person"); err != nil {
		return err
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}

	req := api.CreateCounterparty{Name: name, Type: *kind}
	if *icon != "" {
		req.Icon = icon
	}
	cp, err := s.client.CreateCounterparty(ctx, req)
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(cp)
	}
	fmt.Printf("%s контрагент %s %s\n", output.Green("создан"), cp.Name, output.Dim(cp.ID))
	return nil
}

func typeLabel(t string) string {
	switch t {
	case "organization":
		return "организация"
	case "person":
		return "физлицо"
	default:
		return t
	}
}

const templatesHelp = `finances-kai templates [add <имя>|rm <имя>] — шаблоны платежей

  finances-kai templates
  finances-kai templates add "обед" --amount 450 --tag еда --paid
  finances-kai templates add "аренда" --cp "Арендодатель"
  finances-kai templates add "премия" --type income
  finances-kai templates rm обед

Создать платёж по шаблону: finances-kai new <шаблон> [сумма]`

func cmdTemplates(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "add":
			return cmdTemplateAdd(ctx, args[1:])
		case "rm", "delete":
			return cmdTemplateRemove(ctx, args[1:])
		}
	}

	fs, profile, asJSON := newFlagSet("templates", templatesHelp)
	if err := fs.Parse(args); err != nil {
		return errParsed
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	templates, err := s.client.Templates(ctx)
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(templates)
	}
	if len(templates) == 0 {
		fmt.Println("Шаблонов пока нет")
		return nil
	}

	t := output.NewTable("ШАБЛОН", "ТИП", "СУММА", "ОПИСАНИЕ").RightAlign(2)
	for _, tpl := range templates {
		amount := ""
		if a := strings.TrimSpace(deref(tpl.Amount)); a != "" {
			amount = output.Amount(a, deref(tpl.Currency))
		}
		t.Add(tpl.Name, kindLabel(deref(tpl.Type)), amount, deref(tpl.Description))
	}
	t.Render(os.Stdout)
	return nil
}

func cmdTemplateAdd(ctx context.Context, args []string) error {
	fs, profile, asJSON := newFlagSet("templates add", templatesHelp)
	// Направление и сумма — разными флагами. Один флаг с необязательным
	// значением здесь не сделать: пакет flag таких не умеет и в
	// `--expense --tag еда` принял бы «--tag» за сумму.
	kind := fs.String("type", "expense", "expense|income")
	amount := fs.String("amount", "", "сумма без знака; можно не задавать")
	currency := fs.String("currency", "", "RUB|USD|KZT")
	description := fs.String("description", "", "описание платежа")
	paid := fs.Bool("paid", false, "платежи по шаблону сразу оплачены")
	var tags, cps stringList
	fs.Var(&tags, "tag", "тег; флаг можно повторять")
	fs.Var(&cps, "cp", "контрагент; флаг можно повторять")

	name, err := parseWithName(fs, args)
	if err != nil {
		return err
	}
	if err := checkEnum("--type", *kind, "expense", "income"); err != nil {
		return err
	}
	if err := checkEnum("--currency", *currency, "RUB", "USD", "KZT"); err != nil {
		return err
	}

	req := api.CreateTemplate{Name: name, Status: statusOf(*paid), Type: kind}
	if *amount != "" {
		if _, err := strconv.ParseFloat(*amount, 64); err != nil {
			return fmt.Errorf("--amount: %q не число", *amount)
		}
		// Минус в шаблоне сломал бы `new`: там знак добавляется по type, и
		// расход с минусом в шаблоне стал бы доходом.
		req.Amount = strPtr(strings.TrimPrefix(*amount, "-"))
	}
	if *currency != "" {
		req.Currency = currency
	}
	if *description != "" {
		req.Description = description
	}

	s, err := open(*profile)
	if err != nil {
		return err
	}
	if req.TagIDs, req.CounterpartyIDs, err = s.resolveFilters(ctx, tags, cps); err != nil {
		return err
	}

	tpl, err := s.client.CreateTemplate(ctx, req)
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(tpl)
	}
	fmt.Printf("%s шаблон %s %s\n", output.Green("создан"), tpl.Name, output.Dim(tpl.ID))
	fmt.Printf("применить: finances-kai new %s\n", output.Bold(tpl.Name))
	return nil
}

func cmdTemplateRemove(ctx context.Context, args []string) error {
	fs, profile, _ := newFlagSet("templates rm", templatesHelp)
	yes := fs.Bool("yes", false, "не спрашивать подтверждения")
	name, err := parseWithName(fs, args)
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

	if !*yes {
		fmt.Printf("Удалить шаблон %s?\n", output.Bold(tpl.Name))
		ok, err := confirm("Введите y для подтверждения: ")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("отменено")
			return nil
		}
	}

	if err := s.client.DeleteTemplate(ctx, tpl.ID); err != nil {
		return err
	}
	fmt.Println(output.Yellow("удалён шаблон"), tpl.Name)
	return nil
}

func kindLabel(t string) string {
	switch t {
	case "expense":
		return output.Red("расход")
	case "income":
		return output.Green("доход")
	default:
		return output.Dim("—")
	}
}
