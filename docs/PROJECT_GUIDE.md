# Карта проекта и руководство по рефакторингу

Документ сверён с состоянием проекта после rebase на `origin/main` commit
`fef5b6b` (2026-08-28). Это исходная точка для рефакторинга, а не описание
желаемой архитектуры.

## 1. Назначение

Сервис выполняет четыре задачи:

1. Подключается к Modbus-устройству по URL, поддерживаемому
   `github.com/simonvetter/modbus`.
2. Периодически читает holding-регистры и хранит последние значения тегов.
3. Публикует значения как Prometheus-метрики и JSON.
4. Позволяет записывать уставки через HTTP и Telegram, а также получает
   показания внешних датчиков из Node-RED для Telegram-команды.

Основной сценарий из примера конфигурации — мониторинг температур, состояний
сервоприводов и управление уставками отопления.

## 2. Технологии и проверенные команды

- Go-модуль объявляет версию Go `1.25` в `go.mod`; Docker и GitHub Actions
  используют ту же minor-версию.
- Анализ выполнен с Go `1.26.2 darwin/arm64`.
- Modbus: `github.com/simonvetter/modbus v1.6.0`.
- Метрики: `github.com/VictoriaMetrics/metrics v1.24.0`.
- Telegram: `github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1`.
- Конфигурация: YAML через `gopkg.in/yaml.v2`.

Команды, проверенные в текущем состоянии:

```bash
go test -race ./... -count=1
go vet ./...
go build ./...
```

Команды компилируют все четыре пакета. Тестовая команда сообщает
`[no test files]` для каждого пакета: исполняющих поведение тестов пока нет.
Форматирование проверяется командой `gofmt -l .`.

## 3. Карта файлов

| Путь | Ответственность |
| --- | --- |
| `modbus2prometheus.go` | Флаги, сборка зависимостей, запуск Controller, Telegram и HTTP. |
| `config.go` | YAML-модели, чтение конфигурационного файла. |
| `handlers.go` | Экспорт глобального реестра VictoriaMetrics. |
| `controller/controller.go` | Соединение Modbus, цикл опроса, запись регистров, счётчики. |
| `controller/tags.go` | Модель тега и его последнее значение. |
| `controller/helpers.go` | Разбор операций, проверки типа и строковое представление значения. |
| `controller/json.go` | Снимок тегов для `/tags`. |
| `controller/http.go` | HTTP-обработчики чтения и записи. |
| `telegram/bot.go` | Long polling Telegram, allowlist владельцев, состояние диалога. |
| `telegram/commands.go` | Интерфейс команды и простая реализация. |
| `telegram/commands/sust.go` | Интерактивная запись уставок. |
| `telegram/commands/sens.go` | Чтение данных Node-RED и их форматирование. |
| `etc/modbus2prometheus.config.yaml` | Рабочий пример Modbus-тегов и Telegram. |
| `etc/vmagent.scrape.config.yaml` | Пример scrape-конфигурации vmagent. |
| `etc/systemd/system/*.service` | Примеры systemd units для сервиса и vmagent. |
| `Makefile` | Локальные команды build, test, lint, tidy и clean. |
| `docker/docker-compose.yml` | Контейнеры приложения и nginx proxy. |
| `docker/Dockerfile*` | Сборка приложения и образ nginx. |
| `docker/nginx.conf` | Reverse-proxy routes к приложению и сервисам на Docker host. |
| `.github/workflows/*.yml` | Проверка Go-пакетов и сборка release artifact для Linux ARM. |
| `docs/architecture.mmd` | Исходник Mermaid-карты, встроенной в README. |

Папка `build/` игнорируется Git и может содержать локальный бинарник. Она не
является источником истины для анализа.

## 4. Запуск и жизненный цикл

Текущая последовательность запуска:

```text
main
 ├─ ParseFlags
 │   ├─ проверить путь к YAML
 │   ├─ декодировать Config
 │   └─ применить значения по умолчанию и CLI overrides
 ├─ initController
 │   ├─ создать и открыть ModbusClient
 │   ├─ добавить теги и зарегистрировать gauges
 │   └─ запустить Controller.Poll в goroutine
 ├─ initTelegram
 │   ├─ создать команды
 │   └─ запустить Telegram long polling в goroutine
 └─ http.ListenAndServe
     ├─ /tags
     ├─ /metrics
     └─ /api/v1/write
```

Существенная деталь: `initController` выполняет `defer ctrl.Close()` внутри
самой функции сразу после запуска goroutine. Поэтому `Close` вызывается при
возврате из `initController`, а не при завершении `main`. Одновременно `Poll`
без синхронизации записывает `exit = false`. Результат зависит от порядка
планирования goroutine: опрос либо продолжится, либо завершится с `os.Exit(2)`.

Graceful shutdown отсутствует. Завершение цикла опроса закрывает Modbus и
непосредственно завершает весь процесс с кодом `2`.

В Docker Compose конфигурация хоста монтируется в `/app/config.yaml`, а
приложение и nginx находятся в общей сети `proxy`. Порт приложения `9101` не
публикуется напрямую на host: nginx слушает host port `80`, удаляет префикс
`/modbus2prometheus/` и передаёт запрос приложению. Остальные nginx routes ведут
к Grafana, VictoriaMetrics, Node-RED и Zigbee2MQTT на Docker host.

## 5. Модель тега и операции

Тег задаётся полями:

- `name` — одновременно внутреннее имя и имя Prometheus-метрики;
- `desc` — отображаемое имя Telegram;
- `address` — начальный Modbus-адрес;
- `group` — используется командами `state` и `ust`;
- `operation` — строка с флагами чтения/записи.

Поддерживаемые подстроки `operation`:

| Флаг | Чтение/запись | Go-тип | Modbus-вызов |
| --- | --- | --- | --- |
| `read_uint` | чтение | `uint16` | `ReadRegister(..., HOLDING_REGISTER)` |
| `read_float` | чтение | `float32` | `ReadFloat32(..., HOLDING_REGISTER)` |
| `write_uint` | запись | `uint16` | `WriteRegister(...)` |
| `write_float` | запись | `float32` | `WriteFloat32(...)` |

Флаги объединяются, например `read_uint|write_uint`. Парсер ищет подстроки, а
не разбирает строгую грамматику. Конфликтующие комбинации не запрещены; при
одновременных `uint` и `float` ветка `uint` имеет приоритет.

При добавлении тега регистрируется gauge с именем `tag.Name`. Библиотека
VictoriaMetrics вызывает panic для недопустимого или повторного имени, поэтому
уникальность и синтаксис имён должны проверяться до создания Controller.

## 6. Потоки данных

### Чтение Modbus

```text
Controller.Poll
 → последовательно обойти tags
 → подождать read-period
 → прочитать holding register
 → увеличить req_counter или err_counter
 → обновить Tag.LastValue только при изменении
 → после полного прохода подождать polling-time
```

Значение `float32` занимает два 16-битных регистра; адрес следующего тега
должен учитывать это согласно карте регистров устройства.

Сам `simonvetter/modbus` сериализует `Open`, `Close`, чтение и запись внутренним
mutex. При замене библиотеки эта гарантия должна стать явной частью интерфейса
Controller.

### Prometheus и JSON

- `/metrics` вызывает глобальный `metrics.WritePrometheus(w, true)`.
- Для каждого тега создаётся метрика с именем тега и текущим значением.
- `req_counter` считает успешные попытки вызова чтения, включая чтения,
  вернувшие ошибку, потому что увеличивается сразу после вызова.
- `err_counter` считает ошибки чтения.
- `/tags` возвращает `req_count`, `err_count` и массив `{name,address,value}`.

### Запись

```text
HTTP POST /api/v1/write           Telegram /sust
 → найти Tag                       → выбрать Tag группы ust
 → проверить флаг write_*          → разобрать текст как float64
 → привести float64 к типу тега    → Controller.WriteTag
 → записать holding register
```

Обе точки входа сходятся в `Controller.WriteTag`. Сейчас этот метод не
проверяет конечность числа, целочисленность, диапазон типа и допустимый
доменно-конфигурационный диапазон.

### Внешние датчики

Команда `/sens_th` выполняет GET на
`telegram.nodeRedUrl + "/current_th"` с timeout 10 секунд и ожидает JSON-массив:

```json
[
  {
    "name": "room",
    "data": {
      "battery": 75,
      "humidity": 45.2,
      "pressure": 748.1,
      "temperature": 22.4
    }
  }
]
```

## 7. Фактическая конфигурация

### CLI

| Флаг | Default | Назначение |
| --- | --- | --- |
| `-config` | `./config.yaml` | Путь к YAML. |
| `-httpListenAddr` | `:9101` | Адрес HTTP; пустой host означает все интерфейсы. |
| `-modbusTcpAddr` | `rtuovertcp://192.168.1.200:8899` | Fallback, если `device-url` пуст. |
| `-maxAttempts` | `100` | Передаётся Controller как лимит переподключений. |
| `-botApiToken` | пусто | Перекрывает `telegram.apiToken`, если задан. |

### YAML

| Поле | Default из кода | Примечание |
| --- | --- | --- |
| `device-url` | CLI `-modbusTcpAddr` | URL транспорта Modbus. |
| `device-id` | `16` | Unit ID. |
| `speed` | `19200` | Скорость RTU. |
| `timeout` | `1s` | Timeout Modbus. |
| `polling-time` | `1s` | Пауза между полными проходами. |
| `read-period` | `10ms` | Пауза перед чтением каждого тега. |
| `tags` | пустой список | Карта регистров. |
| `telegram.apiToken` | пусто | Фактически обязателен: Telegram запускается всегда. |
| `telegram.owners` | пустая map | Allowlist Telegram user ID для сообщений. |
| `telegram.nodeRedUrl` | пусто | Нужен для `/sens_th`. |

`ErrTimeout=500ms` задаётся только во внутренней конфигурации Controller.
`MaxAttempts=20` там же перекрывается CLI-значением `100` при штатном запуске.

## 8. Внешние интерфейсы

### HTTP

| Endpoint | Текущее поведение |
| --- | --- |
| `GET /tags` | JSON-снимок тегов; обработчик фактически принимает любой метод. |
| `GET /metrics` | Prometheus exposition format и process metrics. |
| `POST /api/v1/write` | JSON `{"name":"tag","value":42}`; выполняет запись. |

Для методов, отличных от POST, `/api/v1/write` сейчас молча возвращает `200`.
Авторизации, ограничения размера body и серверных timeout нет.

### Telegram

| Команда | Назначение |
| --- | --- |
| `/state_all` | Все последние значения. |
| `/state` | Теги группы `state`. |
| `/ust` | Теги группы `ust`. |
| `/sust` | Интерактивная запись уставки. |
| `/sens_th` | Данные температуры/влажности/батареи из Node-RED. |

`BotCommand.Command` в используемой библиотеке допускает только lowercase,
цифры и underscore. Код передаёт имена с ведущим `/`, поэтому регистрация меню
команд Telegram может отклоняться, хотя ручная обработка сообщений остаётся.

## 9. Результаты анализа

### Критические

1. **Гонка жизненного цикла Controller.**
   `modbus2prometheus.go:initController` откладывает `Close` до возврата из
   `initController`, а `Controller.Poll` конкурентно сбрасывает `exit`. Возможен
   немедленный `os.Exit(2)` либо игнорирование `Close`.
2. **Неаутентифицированное управление.**
   HTTP по умолчанию слушает все интерфейсы, а `/api/v1/write` без авторизации
   пишет регистры отопления. Это критично за пределами строго изолированной сети.
3. **Некорректный Telegram-ввод записывает ноль.**
   В `UstCommand.Action` ошибка `strconv.ParseFloat` меняет текст ответа, но не
   прерывает выполнение. `WriteTag` вызывается с нулевым `val`, а исходная
   ошибка затем перезаписывается.

### Важные

1. `Close`/`Poll`, чтение `LastValue` из Telegram и возврат внутреннего слайса
   из `Tags()` не образуют согласованной модели синхронизации. `go test -race`
   не может это проверить без исполняющих сценарий тестов.
2. В `SensorsCommand.Callback` после ошибки GET используется `resp.Body` без
   проверки `resp != nil`, что способно вызвать panic. HTTP status также не
   проверяется.
3. Детальный ответ датчиков выводит `Humidity` в строке давления вместо
   `Pressure`.
4. Состояние Telegram-диалога (`currentCommand`, `currentTag`) общее для всех
   владельцев и чатов. Два одновременных диалога вмешиваются друг в друга;
   CallbackQuery отдельно по allowlist не проверяется.
5. `Controller.WriteTag` преобразует `float64` в `uint16`/`float32` без
   проверки NaN, Inf, дробной части, переполнения и безопасного диапазона тега.
6. При постоянной ошибке `modbusClient.Open()` счётчик `failAttempts` не
   увеличивается, поэтому `MaxAttempts` не ограничивает такие попытки.
7. `Poll` вызывает `os.Exit(2)`, `telegram.New` вызывает panic при ошибке
   авторизации, а закрытие канала updates приводит к `log.Fatal`. Внутренние
   компоненты управляют жизнью всего процесса и плохо тестируются.
8. Telegram запускается даже при пустом token. Нельзя использовать сервис как
   exporter без Telegram.
9. Конфигурация не проверяет неизвестные YAML-поля, пустой URL, повторные теги,
   конфликтующие операции, положительность duration и корректность metric name.
   Открытый YAML-файл не закрывается.
10. У `http.Server` не заданы `ReadHeaderTimeout`, `ReadTimeout`,
    `WriteTimeout` и graceful shutdown.
11. Глобальный реестр метрик затрудняет изоляцию тестов и нескольких экземпляров
    Controller в одном процессе; общие имена `req_counter`/`err_counter` могут
    конфликтовать.
12. Docker nginx публикует приложение через `/modbus2prometheus/`, включая
    неаутентифицированный write endpoint. Ограничение доверенной сетью остаётся
    обязательным до появления постоянной схемы авторизации.

### Неблокирующие

- Опечатка `TagsHahdler`, непоследовательные `ust`/`sust` и mixed-language
  имена усложняют навигацию.
- Неиспользуемые `logger`, `curChatId`, фактически неиспользуемая логика
  `SensorsCommand.currentVar` и закомментированные блоки создают шум.
- `interface{}` в `Tag.LastValue` и `Action` переносит ошибки типов в runtime.
- HTTP error body объявлен как JSON, но содержит plain text.

## 10. Что уже сделано удачно

- Репозиторий небольшой, а Modbus, HTTP и Telegram уже разделены по пакетам.
- Одна конфигурация тегов служит источником для polling, JSON, метрик и команд.
- Для доступа к Telegram-сообщениям существует allowlist владельцев.
- Внешний HTTP-клиент Node-RED имеет timeout 10 секунд.
- JSON-снимок и callbacks gauges берут `RWMutex` Controller.
- Версии прямых зависимостей зафиксированы в `go.mod`/`go.sum`.
- Makefile, Docker Compose и systemd покрывают основные варианты сборки и
  запуска; GitHub Actions выполняет build/test и собирает Linux ARM release.

Эти свойства стоит сохранить, но сделать границы компонентов явными и
тестируемыми.

## 11. Целевая архитектура

Рекомендуемое направление, без обязательного одномоментного перемещения файлов:

```text
cmd/modbus2prometheus/       composition root, flags, signals, exit codes
internal/config/             YAML + defaults + validation
internal/domain/             Tag, Value, Operation, write constraints
internal/controller/         polling, cache, reconnect policy
internal/modbus/             adapter around simonvetter/modbus
internal/httpapi/            handlers, auth, DTO, server timeouts
internal/telegram/           bot adapter and per-chat conversations
internal/metrics/            isolated registry and metric naming
```

Направление зависимостей:

```text
HTTP ───────┐
Telegram ───┼─→ domain service ─→ Modbus interface ─→ library adapter
Poller ─────┘          │
                      └─→ value store ─→ HTTP read / metrics / Telegram read
```

Процесс (`main`) должен собирать компоненты и определять политику завершения.
Ни Controller, ни Telegram, ни HTTP handler не должны вызывать `os.Exit`,
`log.Fatal` или panic для ожидаемых внешних ошибок.

## 12. Инварианты рефакторинга

До появления регрессионных тестов нельзя менять одновременно структуру и
поведение. Для каждого небольшого изменения действует цикл:

1. Зафиксировать текущее или требуемое поведение тестом.
2. Убедиться, что тест падает по ожидаемой причине, если это новый контракт.
3. Внести минимальное изменение.
4. Запустить тест пакета, затем `go test -race ./...`, `go vet ./...` и сборку.
5. Проверить diff на изменение Modbus-адресов, типов и metric names.

Без явного решения владельца проекта нельзя менять:

- `device-id`, регистры и соответствие `uint16`/`float32`;
- endianness/word order библиотеки Modbus;
- имена существующих метрик и JSON-поля;
- группы и имена Telegram-команд;
- timing и reconnect policy;
- значения уставок и допустимые диапазоны.

## 13. Этапы рефакторинга

### Этап 0. Защитить текущее управление

- Исправить lifecycle race и убрать `os.Exit` из Controller.
- Запретить запись после ошибки разбора Telegram-ввода.
- Обработать ошибку Node-RED без panic и исправить давление.
- Ограничить HTTP-запись localhost до выбора постоянной аутентификации.
- Добавить серверные timeout.

### Этап 1. Создать тестовый шов вокруг Modbus

- Ввести минимальный интерфейс только для используемых методов клиента.
- Подставлять fake client в тестах.
- Покрыть чтение `uint16`/`float32`, запись, reconnect и cancel.
- Использовать отдельный VictoriaMetrics `Set` на экземпляр Controller.

Подробный план этого этапа находится в `docs/REFACTORING_PLAN.md`.

### Этап 2. Валидировать домен и конфигурацию

- Сделать операции строгим типом и отвергать конфликтующие флаги.
- Проверять уникальность имён, metric syntax, адреса и duration.
- Добавить для writable-тегов обязательные безопасные `min`/`max` из карты
  конкретного оборудования.
- Сделать Telegram опциональным и проверять его секцию только при включении.

### Этап 3. Разделить adapters и application logic

- Перенести side effects из команд/handlers в сервис управления тегами.
- Сделать HTTP и Telegram тонкими adapters с одинаковыми правилами записи.
- Хранить Telegram conversation state по паре `(chatID, userID)`.
- Возвращать typed errors, которые adapters переводят в HTTP/Telegram ответы.

### Этап 4. Исправить эксплуатационный контракт

- Поддерживать единый порт `9101` в приложении, vmagent, nginx и systemd.
- Поддерживать README и Mermaid-карту синхронно с deployment-файлами.
- Добавить health/readiness endpoints и метрики reconnect/write failures.
- Усилить существующий CI: format check, `go test -race` и `go vet` в дополнение
  к build/test; не допускать расхождения версии Go с `go.mod`.

## 14. Решения, нужные перед изменением поведения

1. Где доступен HTTP: только localhost, домашняя LAN или через reverse proxy?
2. Нужна ли запись через HTTP вообще; если да, bearer token, mTLS или защита
   только на reverse proxy?
3. Каковы допустимые `min`, `max` и шаг для каждого writable-регистра?
4. Должны ли несколько Telegram-владельцев вести диалоги одновременно?
5. Является ли `os.Exit(2)` сигналом для обязательного systemd restart после
   исчерпания reconnect attempts или сервис должен оставаться доступным для
   диагностики?
6. Какие варианты Raspberry Pi поддерживать: ARMv6, ARMv7 и/или ARM64?

Рекомендуемый следующий шаг — согласовать пункты 1–3, затем выполнить первый
план из `docs/REFACTORING_PLAN.md` без массового перемещения пакетов.
