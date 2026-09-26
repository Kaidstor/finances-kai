// Package command — CLI-слой: роутер, разбор флагов и сами команды.
package command

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Kaidstor/finances-kai/internal/exit"
	"github.com/Kaidstor/finances-kai/internal/output"
)

const usage = `finances-kai — учёт доходов и расходов из терминала

Использование:
  finances-kai <команда> [аргументы]

Платежи:
  ls [фильтры]                список платежей за период
  add <сумма> [описание]      создать платёж; сумма с минусом — расход
  new <шаблон> [сумма]        создать платёж по шаблону
  show <id>                   карточка платежа
  edit <id> [поля]            изменить платёж
  rm <id>                     удалить платёж
  attach <id> <файл>…         приложить файлы (чеки, счета)

Аналитика:
  stats [фильтры]             сводка по тегам за период
  forecast [--months N]       прогноз трат по подпискам

Справочники:
  tags [add <имя>]            теги
  cp [add <имя>]              контрагенты (полное имя: counterparties)
  templates [add|rm]          шаблоны

Прочее:
  export [набор] [фильтры]    zip со счетами; набор фильтров можно сохранить
  login                       сохранить API-токен профиля
  profile [use|add|rm]        профили подключения
  doctor                      настройки, источник токена с маской, связь
  version, --version          версия

Фильтры ls и export:
  --period today|week|month|year|all   период (по умолчанию month)
  --from YYYY-MM-DD --to YYYY-MM-DD    явные границы, отменяют --period
  --tag <имя>                          тег; флаг можно повторять
  --cp <имя>                           контрагент; флаг можно повторять
  --type income|expense                только доходы или только расходы

Общие флаги:
  --profile <имя>   профиль вместо текущего
  --json            конверт {v, command, exit, data, warning, error} в stdout,
                    отказ тоже; флаг ставится в любом месте строки
  --human           текст (по умолчанию)
  -h, --help        справка по команде

Настройки — ~/.config/finances-kai/config.json: профили (url, token_ref) и
наборы export. Токен ищется так: FINANCES_KAI_TOKEN → sec get <token_ref> →
системное хранилище (Keychain, libsecret); в argv он не попадает.

Переменные окружения:
  FINANCES_KAI_URL, FINANCES_KAI_TOKEN, FINANCES_KAI_TOKEN_REF,
  FINANCES_KAI_PROFILE, FINANCES_KAI_HOME

Коды выхода:
  0 сделано
  1 приложение ответило, но не применило (отвергло поле, внутренняя ошибка)
  2 ошибка аргументов, настроек или токена; нужен --yes; нет связи
  3 не найдено: платёж, тег, контрагент, шаблон, профиль, набор; export без платежей
  4 приложение не ответило в срок
  130 прервано (Ctrl+C)`

// Run разбирает argv и возвращает код выхода. Ошибки печатает сам — наверх
// уходит только число.
func Run(args []string) int {
	asJSON, args := splitGlobalFlags(args)

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(usage)
		return exit.OK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p := output.New(asJSON)
	cmd, rest := args[0], args[1:]
	p.Command = cmd

	handlers := map[string]func(context.Context, *output.Printer, []string) error{
		"ls": cmdList, "list": cmdList,
		"add":  cmdAdd,
		"new":  cmdNew,
		"show": cmdShow,
		"edit": cmdEdit,
		"rm":   cmdRemove, "delete": cmdRemove,
		"attach":   cmdAttach,
		"stats":    cmdStats,
		"forecast": cmdForecast,
		"tags":     cmdTags,
		"cp":       cmdCounterparties, "counterparties": cmdCounterparties,
		"templates": cmdTemplates,
		"export":    cmdExport,
		"login":     cmdLogin,
		"profile":   cmdProfile, "profiles": cmdProfile,
		"doctor": cmdDoctor,
	}

	var err error
	switch h, ok := handlers[cmd]; {
	case ok:
		err = h(ctx, p, rest)
	case cmd == "version" || cmd == "--version":
		p.Command = "version"
		if p.JSON {
			err = p.Data(map[string]string{"version": version})
		} else {
			fmt.Println(version)
		}
	default:
		p.Command = ""
		if !p.JSON {
			fmt.Fprintf(os.Stderr, "неизвестная команда %q\n\n%s\n", cmd, usage)
		}
		err = &cliError{code: exit.Tool, kind: "usage", printed: true,
			err: fmt.Errorf("неизвестная команда %q; finances-kai --help покажет список", cmd)}
	}

	if err != nil {
		return fail(p, err)
	}
	if p.JSON && !p.Emitted() {
		_ = p.Data(nil)
	}
	return exit.OK
}

// version подставляется линкером: -X …/internal/command.version=…
var version = "dev"

// splitGlobalFlags вынимает --json и --human из любого места argv: подкоманды
// разбирают остаток своим FlagSet, и флаг после позиционного аргумента иначе
// потерялся бы. Последний из двух побеждает.
func splitGlobalFlags(args []string) (asJSON bool, rest []string) {
	for i, a := range args {
		if a == "--" {
			return asJSON, append(rest, args[i:]...)
		}
		switch a {
		case "--json":
			asJSON = true
		case "--human":
			asJSON = false
		default:
			rest = append(rest, a)
		}
	}
	return asJSON, rest
}
