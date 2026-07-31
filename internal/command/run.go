// Package command — CLI-слой: роутер, разбор флагов и сами команды.
package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Kaidstor/finances-next/cli/internal/api"
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
  export [фильтры]            zip со счетами за период
  login                       сохранить API-токен профиля
  profile [use|add|rm]        профили подключения
  doctor                      проверить профиль, токен и связь

Фильтры ls и export:
  --period today|week|month|year|all   период (по умолчанию month)
  --from YYYY-MM-DD --to YYYY-MM-DD    явные границы, отменяют --period
  --tag <имя>                          тег; флаг можно повторять
  --cp <имя>                           контрагент; флаг можно повторять
  --type income|expense                только доходы или только расходы

Общие флаги:
  --profile <имя>   профиль вместо текущего
  --json            машиночитаемый вывод
  -h, --help        справка по команде

Переменные окружения:
  FINANCES_KAI_URL, FINANCES_KAI_TOKEN, FINANCES_KAI_PROFILE, FINANCES_KAI_HOME`

// Run разбирает argv и возвращает код выхода. Ошибки печатает сам — наверх
// уходит только число.
func Run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(usage)
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "ls", "list":
		err = cmdList(ctx, rest)
	case "add":
		err = cmdAdd(ctx, rest)
	case "new":
		err = cmdNew(ctx, rest)
	case "show":
		err = cmdShow(ctx, rest)
	case "edit":
		err = cmdEdit(ctx, rest)
	case "rm", "delete":
		err = cmdRemove(ctx, rest)
	case "attach":
		err = cmdAttach(ctx, rest)
	case "stats":
		err = cmdStats(ctx, rest)
	case "forecast":
		err = cmdForecast(ctx, rest)
	case "tags":
		err = cmdTags(ctx, rest)
	case "cp", "counterparties":
		err = cmdCounterparties(ctx, rest)
	case "templates":
		err = cmdTemplates(ctx, rest)
	case "export":
		err = cmdExport(ctx, rest)
	case "login":
		err = cmdLogin(ctx, rest)
	case "profile", "profiles":
		err = cmdProfile(ctx, rest)
	case "doctor":
		err = cmdDoctor(ctx, rest)
	case "version":
		fmt.Println(version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда %q\n\n%s\n", cmd, usage)
		return 2
	}

	if err != nil {
		return fail(err)
	}
	return 0
}

// version подставляется линкером: -X …/internal/command.version=…
var version = "dev"

func fail(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	if err.Error() == "" {
		return 2 // разбор флагов уже всё напечатал
	}
	fmt.Fprintln(os.Stderr, "ошибка:", err)
	if errors.Is(err, api.ErrUnauthorized) {
		fmt.Fprintln(os.Stderr, "подсказка: токен истёк или отозван — выпустите новый в /settings и выполните `finances-kai login`")
	}
	return 1
}
