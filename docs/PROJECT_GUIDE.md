# Карта проекта и руководство по рефакторингу

Документ сверён с состоянием ветки после первой итерации безопасного
рефакторинга (2026-08-29). Разделы о текущем поведении описывают реализованный
контракт; нерешённые вопросы перечислены отдельно.

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
- Проверки выполнены с Go `1.25.4 darwin/arm64`.
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

Команды компилируют все четыре пакета. Исполняющие тесты покрывают lifecycle и
reconnect Controller, snapshot isolation, HTTP write contract, server timeouts
и Telegram-команды `sens`/`sust`; пакет `telegram` пока не имеет тестов.
Форматирование проверяется командой `gofmt -l .`, которая не должна выводить
файлов.

## 3. Карта файлов

| Путь | Ответственность |
| --- | --- |
| `modbus2prometheus.go` | Флаги, сборка зависимостей, запуск Controller, Telegram и HTTP. |
| `config.go` | YAML-модели, чтение конфигурационного файла. |
| `handlers.go` | Экспорт глобального реестра VictoriaMetrics. |
| `controller/client.go` | Минимальный интерфейс Modbus-клиента и production factory. |
| `controller/controller.go` | Соединение Modbus, цикл опроса, запись регистров, счётчики. |
| `controller/tags.go` | Модель тега и его последнее значение. |
| `controller/helpers.go` | Разбор операций, проверки типа и строковое представление значения. |
| `controller/json.go` | Снимок тегов для `/tags`. |
| `controller/http.go` | HTTP-обработчики чтения и записи. |
| `controller/*_test.go` | Fake Modbus client и regression-тесты Controller/HTTP. |
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
 │   └─ добавить теги и зарегистрировать gauges
 ├─ signal.NotifyContext(SIGINT, SIGTERM)
 ├─ Controller.Run(ctx) в goroutine
 ├─ initTelegram
 │   ├─ создать команды
 │   └─ запустить Telegram long polling в goroutine
 └─ http.Server.ListenAndServe в goroutine
     ├─ /tags
     ├─ /metrics
     └─ /api/v1/write
```

`main` владеет политикой завершения. Сигнал отменяет context Controller,
`Run` возвращает причину остановки и закрывает Modbus-клиент. HTTP server имеет
`ReadHeaderTimeout=5s`, `ReadTimeout=10s`, `WriteTimeout=30s` и
`IdleTimeout=60s`; graceful shutdown ограничен 10 секундами. Ошибка polling или
listener также отменяет остальные компоненты и возвращается из `run`.

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
Controller.Run(ctx)
 → последовательно обойти tags
 → подождать read-period
 → прочитать holding register
 → увеличить req_counter или err_counter
 → обновить Tag.LastValue только при изменении
 → после полного прохода подождать polling-time
```

Значение `float32` занимает два 16-битных регистра; адрес следующего тега
должен учитывать это согласно карте регистров устройства.

Controller зависит от минимального интерфейса семи используемых Modbus-методов.
Production adapter `simonvetter/modbus` сериализует `Open`, `Close`, чтение и
запись внутренним mutex; тесты подставляют fake client.

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
 → привести float64 к типу тега    → Controller.WriteTagByName
 → записать holding register
```

Обе точки входа сходятся в `Controller.WriteTagByName`. Telegram отклоняет
ошибку разбора, `NaN` и `Inf` до записи. Controller пока не проверяет дробную
часть для `uint16`, переполнение и допустимый доменно-конфигурационный диапазон.

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

Transport errors, HTTP status вне `2xx` и ошибки JSON возвращаются в callback и
переводятся в Telegram-сообщение без разыменования пустого response.

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
| `http.writeBearerToken` | пусто | Опциональный Bearer token только для `/api/v1/write`; пустое значение отключает auth. |

`ErrTimeout=500ms` задаётся только во внутренней конфигурации Controller.
`MaxAttempts=20` там же перекрывается CLI-значением `100` при штатном запуске.

## 8. Внешние интерфейсы

### HTTP

| Endpoint | Текущее поведение |
| --- | --- |
| `GET /tags` | JSON-снимок тегов; обработчик фактически принимает любой метод. |
| `GET /metrics` | Prometheus exposition format и process metrics. |
| `POST /api/v1/write` | Строгий JSON `{"name":"tag","value":42}`; успех `204`. |

`/api/v1/write` возвращает JSON error: `400` для некорректного body, `404` для
неизвестного тега, `403` для read-only тега и `502` для ошибки Modbus. Другие
методы получают `405` и `Allow: POST`. Body ограничен 1 MiB, неизвестные JSON
поля запрещены. Если `http.writeBearerToken` задан, запрос должен содержать
`Authorization: Bearer <token>`; иначе возвращаются `401`, JSON error и
`WWW-Authenticate: Bearer`. Пустое или отсутствующее поле сохраняет прежний
режим без авторизации.

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

## 9. Состояние после первой итерации

### Исправлено и покрыто regression-тестами

1. Lifecycle Controller управляется context; polling не вызывает `os.Exit`, а
   `main` выполняет graceful shutdown HTTP и Modbus.
2. Reconnect limit считает каждую неуспешную попытку `Open` и возвращает
   последнюю ошибку после `MaxAttempts`.
3. Controller использует минимальный Modbus-интерфейс, fake client и отдельный
   VictoriaMetrics `Set` на экземпляр.
4. Telegram не записывает текст, `NaN` или `Inf`; Node-RED transport/status/JSON
   errors обрабатываются без panic, а pressure выводится из правильного поля.
5. Presentation adapters читают immutable `TagSnapshot`; race-тест одновременно
   выполняет polling и snapshot reads.
6. HTTP write имеет явные статусы, JSON error body, лимит 1 MiB, строгие поля и
   server timeouts.
7. Для HTTP write добавлена опциональная Bearer-аутентификация с безопасным
   сравнением токена и обратной совместимостью старой конфигурации.

### Критическое нерешённое

1. **Аутентификация отключена по умолчанию для совместимости.** HTTP слушает все
   интерфейсы, а Docker nginx публикует `/api/v1/write`. Пока
   `http.writeBearerToken` не задан, сервис должен работать только в доверенной
   сети.
2. **Нет безопасных диапазонов уставок.** `WriteTagByName` всё ещё преобразует
   `float64` в `uint16`/`float32` без проверки дробной части, переполнения и
   допустимых `min`/`max` конкретного регистра.

### Важное нерешённое

1. Состояние Telegram-диалога общее для всех владельцев и чатов; CallbackQuery
   отдельно по allowlist не проверяется.
2. Telegram запускается даже при пустом token. Ошибка авторизации приводит к
   panic, а закрытие updates channel — к `log.Fatal`.
3. Конфигурация не проверяет неизвестные YAML-поля, пустой URL, повторные теги,
   конфликтующие операции, положительность duration и корректность metric name;
   открытый YAML-файл не закрывается.
4. `ParseOperation` завершает процесс для неизвестной операции вместо возврата
   typed error вызывающему коду.

### Неблокирующее

- Опечатка `TagsHahdler`, непоследовательные `ust`/`sust` и mixed-language
  имена усложняют навигацию.
- Неиспользуемые `logger`, `curChatId`, фактически неиспользуемая логика
  `SensorsCommand.currentVar` и закомментированные блоки создают шум.
- `interface{}` в `Tag.LastValue` и `Action` переносит ошибки типов в runtime.

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

### Этап 0. Защитить текущее управление — частично выполнен

- Lifecycle race исправлен, `os.Exit` удалён из Controller.
- Запись после ошибки разбора Telegram-ввода запрещена.
- Ошибки Node-RED обрабатываются без panic, pressure исправлен.
- Добавлены строгий HTTP contract, body limit и server timeouts.
- Добавлена опциональная Bearer-аутентификация HTTP write; обязательный режим
  пока не включён ради обратной совместимости.

### Этап 1. Создать тестовый шов вокруг Modbus — выполнена основа

- Введён минимальный интерфейс только для используемых методов клиента.
- Fake client используется в lifecycle, snapshot и HTTP tests.
- Покрыты reconnect, cancel, concurrent snapshot и HTTP write; отдельные
  сценарии успешного `float32` polling/write ещё можно добавить.
- Controller использует отдельный VictoriaMetrics `Set`.

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

### Этап 4. Исправить эксплуатационный контракт — частично выполнен

- Приложение, vmagent target, nginx и systemd согласованы на порту `9101`.
- README и deployment paths сверены с файлами репозитория.
- CI запускает format check, `go test -race`, `go vet` и build, а версию Go
  читает из `go.mod`.
- Health/readiness endpoints и метрики reconnect/write failures ещё не добавлены.

## 14. Решения, нужные перед изменением поведения

1. Где доступен HTTP: только localhost, домашняя LAN или через reverse proxy?
2. Следует ли сделать Bearer-аутентификацию HTTP write обязательной и как
   мигрировать существующую конфигурацию?
3. Каковы допустимые `min`, `max` и шаг для каждого writable-регистра?
4. Должны ли несколько Telegram-владельцев вести диалоги одновременно?
5. Какие варианты Raspberry Pi поддерживать: ARMv6, ARMv7 и/или ARM64?

Рекомендуемый следующий шаг — согласовать пункты 1–3, затем отдельно спланировать
обязательный режим auth, domain ranges/config validation и per-chat Telegram
state.
