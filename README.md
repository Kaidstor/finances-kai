# finances-kai

CLI к приложению учёта доходов и расходов
[finances](https://github.com/Kaidstor/finances-next): тот же HTTP-API, что у
веба и raycast-расширения, авторизация по API-токену.

```
$ finances-kai ls --period month
ДАТА                СУММА  ОПИСАНИЕ       ТЕГИ      КОНТРАГЕНТЫ     ID
2026-07-15     -250,00 ₽   кофе           Еда       🛒 Азбука вкуса  11111111
2026-07-05  120 000,00 ₽   зарплата                                 33333333
2026-07-02   -1 200,50 ₽*  Netflix        Подписки  Netflix         22222222

2026-07-01 — 2026-07-31 · 3 записи
доход 120 000,00 ₽ · расход 1 450,50 ₽ · сальдо 118 549,50 ₽
```

Звёздочка у суммы — платёж не оплачен.

## Установка

```bash
brew install kaidstor/tap/finances-kai
```

Из исходников:

```bash
just install          # сборка в ~/.local/bin
just install-skill    # подключить скилл агенту

# без just:
go build -o ~/.local/bin/finances-kai .
```

`just` ставится через `cargo install just` или `brew install just`.

## Первый запуск

Токен выпускается в приложении: **/settings → «API Токены»**.

```bash
finances-kai login --url https://paytracker.ru
# токен запросится с stdin — в аргументах его передавать не нужно,
# оттуда он виден в ps и остаётся в истории шелла

finances-kai doctor   # проверить, что всё сошлось
```

Токен уходит в системное хранилище (Keychain на macOS, libsecret на Linux).
Если хранилища нет, он ложится файлом `~/.config/finances-kai/tokens/<профиль>`
с правами 0600, и `login` об этом предупредит.

## Команды

| | |
|---|---|
| `ls [фильтры]` | платежи за период |
| `add <сумма> [описание]` | создать платёж; минус — расход |
| `new <шаблон> [сумма]` | создать по шаблону |
| `show <id>` | карточка платежа |
| `edit <id> [поля]` | изменить платёж |
| `rm <id>` | удалить платёж |
| `attach <id> <файл>…` | приложить чеки и счета |
| `stats [фильтры]` | сводка по тегам |
| `forecast [--months N]` | прогноз по подпискам |
| `tags`, `tags add <имя>` | теги |
| `cp`, `cp add <имя>` | контрагенты |
| `templates`, `templates add/rm` | шаблоны |
| `export [набор] [фильтры]` | zip со счетами |
| `login`, `profile`, `doctor` | подключение |

### Примеры

```bash
finances-kai add -250 кофе --tag еда --paid
finances-kai add -1500 "подписка" --cp Netflix --currency USD --date yesterday
finances-kai new обед                      # сумма и теги из шаблона
finances-kai new аренда 45000 --date 2026-08-01

finances-kai edit 11111111 --amount -300 --paid
finances-kai edit 11111111 --tag еда --tag кафе    # теги заменяются целиком
finances-kai attach 11111111 ~/Downloads/чек.pdf

finances-kai ls --period year --type expense --tag еда
finances-kai ls --from 2026-01-01 --to 2026-03-31 --json | jq '.[].amount'

finances-kai stats --period year
finances-kai forecast --by-subscription

finances-kai templates add "обед" --amount 450 --tag еда --paid
finances-kai export --period year -o 2026.zip
```

### Сохранённые наборы для экспорта

Набор фильтров сохраняется под именем, период задаётся при запуске — чтобы не
набирать одно и то же каждый квартал:

```bash
finances-kai export --save аренда --type expense --tag аренда --cp Арендодатель
finances-kai export аренда --period year
finances-kai export аренда --from 2026-01-01 --to 2026-06-30

finances-kai export --list
finances-kai export --forget аренда
```

Наборы лежат в `~/.config/finances-kai/config.json`. Теги и контрагенты
хранятся **именами**, а не идентификаторами: набор остаётся читаемым, переживает
пересоздание тега и работает в любом профиле, где есть такие же имена. Если тег
переименовали, набор честно откажет вместо тихой выгрузки не тех данных.

Флаги в момент запуска перекрывают сохранённые. `--period` при этом сбрасывает
сохранённые `--from`/`--to`: явные границы приоритетнее периода, и без сброса
переданный период молча не подействовал бы.

Имя файла приходит от сервера и содержит границы периода
(`invoices_01.01.2026-31.03.2026.zip`), поэтому выгрузки за разные периоды не
затирают друг друга.

### Вложения

`attach` принимает jpg, png, gif, webp, pdf, txt, csv, doc(x), xls(x) до 10 МБ —
тот же список, что и веб-форма. Расширение и размер проверяются до отправки,
MIME-тип проставляется по расширению: сервер сверяет его с белым списком и
`application/octet-stream`, который Go ставит по умолчанию, не принимает.

Вложения попадают в архив `export` вместе со счетами.

### Сводка

`stats` группирует по тегам и считает в базовой валюте пользователя. Платёж с
несколькими тегами попадает в каждый **целиком**, поэтому сумма долей бывает
больше 100%: доля отвечает на «сколько прошло через тег», а не «какая часть
от целого». Платежи без тегов сводятся в строку «без тега».

Теги и контрагенты указываются **именем**, не UUID. Совпадение ищется без учёта
регистра: сначала точное, потом по подстроке. Если под запрос подходит
несколько — команда откажет и перечислит варианты, а не выберет наугад.
Подтег можно назвать хвостом пути: `--tag кафе` найдёт «Еда / кафе».

### Фильтры

```
--period today|week|month|year|all   по умолчанию month
--from YYYY-MM-DD --to YYYY-MM-DD    отменяют --period
--tag <имя>                          повторяемый
--cp <имя>                           повторяемый
--type income|expense
--status paid|unpaid                 только у ls
--search <подстрока>                 только у ls, по описанию
--limit N                            только у ls и stats, 0 — без ограничения
```

`stats` понимает `--period`, `--from`, `--to` и `--income`; теги и контрагентов
он не фильтрует — по ним он и группирует.

`--period week` — текущая неделя с понедельника, `month` и `year` —
календарные. У `ls` отбор идёт на стороне CLI (в `GET /api/payments` фильтров
нет), у `export` — на стороне сервера.

## Профили

```bash
finances-kai profile                                   # список
finances-kai profile add local http://localhost:3000
finances-kai login --profile local                     # токен для него
finances-kai profile use local
finances-kai ls --profile prod                         # разово, не переключаясь
```

## Переменные окружения

| | |
|---|---|
| `FINANCES_KAI_URL` | адрес приложения; работает без сохранённого профиля |
| `FINANCES_KAI_TOKEN` | токен вместо хранилища |
| `FINANCES_KAI_PROFILE` | профиль вместо текущего |
| `FINANCES_KAI_HOME` | каталог конфигурации вместо `~/.config/finances-kai` |
| `NO_COLOR` | отключить цвет |

Цвет выключается и сам, когда вывод уходит в пайп или файл.

## Разработка

```bash
just test     # go vet + go test
just build
```

Устройство: тонкий `main.go`, всё остальное в `internal/` —
`command` (роутер и команды), `api` (HTTP-клиент), `config` (профили),
`keyring` (токены), `output` (таблицы, цвета, форматирование).
Из зависимостей — только стандартная библиотека.

## Релиз

```bash
./release.sh              # patch: 0.1.0 → 0.1.1
./release.sh minor        # или major, или явное v1.2.3
./release.sh -n minor     # dry-run
```

Скрипт проверяет состояние ветки, гоняет `gofmt`/`vet`/`test` и ставит тег.
Пуш тега запускает [release.yml](.github/workflows/release.yml): goreleaser
собирает бинарники (darwin/linux × amd64/arm64), создаёт GitHub Release и
обновляет cask в [kaidstor/homebrew-tap](https://github.com/kaidstor/homebrew-tap).

Для пуша cask в tap нужен secret `HOMEBREW_TAP_GITHUB_TOKEN` — PAT с правом
записи в tap-репозиторий.
