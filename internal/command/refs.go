package command

import (
	"context"
	"fmt"
	"os"
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

const templatesHelp = `finances-kai templates — шаблоны платежей

Создать платёж по шаблону: finances-kai new <шаблон> [сумма]`

func cmdTemplates(ctx context.Context, args []string) error {
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
