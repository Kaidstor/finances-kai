// finances-kai — учёт доходов и расходов из терминала: тот же API, что у
// веб-приложения и raycast-расширения, авторизация по API-токену из /settings.
//
// Точка входа тонкая: вся логика — в пакетах internal/. Карта:
//
//	internal/command  CLI-слой: роутер Run, usage, разбор флагов, все команды
//	internal/api      HTTP-клиент к /api/*: Bearer, конверт {success,data,error}
//	internal/config   профили (url + базовая валюта) в ~/.config/finances-kai
//	internal/keyring  API-токен: env → системное хранилище ОС → файл
//	internal/output   таблицы, цвета, --json, форматирование сумм и дат
package main

import (
	"os"

	"github.com/Kaidstor/finances-next/cli/internal/command"
)

func main() { os.Exit(command.Run(os.Args[1:])) }
