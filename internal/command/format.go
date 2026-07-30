package command

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Kaidstor/finances-next/cli/internal/api"
	"github.com/Kaidstor/finances-next/cli/internal/output"
)

// errParsed — флаги уже напечатали свою диагностику, второй раз печатать нечего.
// Пустой текст: fail() по нему понимает, что своё сообщение добавлять не надо.
var errParsed = errors.New("")

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func describe(p api.Payment) string {
	if d := strings.TrimSpace(deref(p.Description)); d != "" {
		return d
	}
	return output.Dim("(без описания)")
}

// colorAmount красит сумму по знаку и помечает неоплаченные — расход, который
// ещё предстоит, иначе неотличим от уже потраченного.
func colorAmount(p api.Payment) string {
	s := output.Amount(p.Amount, p.Currency)
	v, err := strconv.ParseFloat(p.Amount, 64)
	switch {
	case err != nil:
	case v < 0:
		s = output.Red(s)
	default:
		s = output.Green(s)
	}
	if p.Status == "unpaid" {
		s += output.Yellow("*")
	}
	return s
}

// amountCell — сумма для таблицы. Место под пометку занято всегда: иначе
// звёздочка вылезает за правый край колонки и ломает выравнивание соседних сумм.
func amountCell(p api.Payment) string {
	if p.Status == "unpaid" {
		return colorAmount(p)
	}
	return colorAmount(p) + " "
}

func statusLabel(status string) string {
	switch status {
	case "paid":
		return output.Green("оплачен")
	case "unpaid":
		return output.Yellow("не оплачен")
	default:
		return status
	}
}

func statusOf(paid bool) string {
	if paid {
		return "paid"
	}
	return "unpaid"
}

func joinTags(tags []api.Tag) string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

func joinCounterparties(items []api.Counterparty) string {
	names := make([]string, 0, len(items))
	for _, c := range items {
		if icon := strings.TrimSpace(deref(c.Icon)); icon != "" {
			names = append(names, icon+" "+c.Name)
			continue
		}
		names = append(names, c.Name)
	}
	return strings.Join(names, ", ")
}

// shortID — префикс UUID, которого хватает, чтобы адресовать платёж в rm и show.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func matchesKind(p api.Payment, kind string) bool {
	if kind == "" {
		return true
	}
	v, err := strconv.ParseFloat(p.Amount, 64)
	if err != nil {
		return false
	}
	if kind == "income" {
		return v >= 0
	}
	return v < 0
}

func matchesSearch(p api.Payment, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(deref(p.Description)), strings.ToLower(needle))
}

func hasAnyTag(p api.Payment, ids []string) bool {
	if len(ids) == 0 {
		return true
	}
	for _, want := range ids {
		for _, t := range p.Tags {
			if t.ID == want {
				return true
			}
		}
	}
	return false
}

func hasAnyCounterparty(p api.Payment, ids []string) bool {
	if len(ids) == 0 {
		return true
	}
	for _, want := range ids {
		for _, c := range p.Counterparties {
			if c.ID == want {
				return true
			}
		}
	}
	return false
}

// splitLeading отделяет позиционные аргументы в голове argv от флагов за ними.
//
// Пакет flag прекращает разбор на первом же позиционном аргументе, поэтому
// `add -250 "кофе" --tag еда` без этой предобработки отдал бы "--tag" и "еда"
// как часть описания, а сам --tag молча не применился бы.
func splitLeading(args []string) (positional, flags []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

// parseWithID разбирает команду вида `<команда> <id> [флаги]` — позиционный
// аргумент отделяется от флагов до Parse, иначе флаги за ним не применятся.
func parseWithID(fs *flag.FlagSet, args []string) (string, error) {
	words, flags := splitLeading(args)
	if err := fs.Parse(flags); err != nil {
		return "", errParsed
	}
	words = append(words, fs.Args()...)
	if len(words) != 1 {
		fs.Usage()
		return "", errParsed
	}
	return words[0], nil
}

// parseWithName — то же для команд вида `<команда> add <имя из нескольких слов>`.
func parseWithName(fs *flag.FlagSet, args []string) (string, error) {
	words, flags := splitLeading(args)
	if err := fs.Parse(flags); err != nil {
		return "", errParsed
	}
	name := strings.TrimSpace(strings.Join(append(words, fs.Args()...), " "))
	if name == "" {
		fs.Usage()
		return "", errParsed
	}
	return name, nil
}

// takeAmount снимает сумму с головы argv до разбора флагов: "-250" для
// flag-пакета неотличимо от неизвестного флага.
func takeAmount(args []string) (amount string, rest []string, err error) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, addHelp)
		return "", nil, errParsed
	}
	if args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stderr, addHelp)
		return "", nil, errParsed
	}
	if _, err := strconv.ParseFloat(args[0], 64); err != nil {
		return "", nil, fmt.Errorf("первым аргументом ожидается сумма, получено %q", args[0])
	}
	return args[0], args[1:], nil
}

func checkEnum(flagName, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%s: ожидается %s, получено %q", flagName, strings.Join(allowed, "|"), value)
}

func plural(n int, one, few, many string) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

// confirm спрашивает подтверждение. Отвечать некому — отказываем с подсказкой
// про --yes, а не «отменено» с нулевым кодом: в скрипте это выглядело бы как
// успешно выполненное удаление.
//
// Проверки на «стандартный ввод — терминал» мало: /dev/null тоже символьное
// устройство, поэтому пустой ответ ловим и на чтении.
func confirm(prompt string) (bool, error) {
	needYes := errors.New("нужно подтверждение, а отвечать некому: повторите с --yes")

	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false, needYes
	}
	fmt.Print(prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return false, needYes
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes" || answer == "д" || answer == "да", nil
}
