# План усиления домена, адаптеров и эксплуатации

**Цель:** закрыть оставшиеся риски конфигурации и записи, изолировать диалоги
Telegram, добавить эксплуатационные пробы и метрики и убрать накопленный
технический долг без изменения Modbus-карты и публичных имён.

**Архитектура:** Controller остаётся прикладным сервисом и единственной
точкой правил чтения/записи. Конфигурация валидируется до открытия Modbus и
запуска горутин; HTTP и Telegram зависят от минимальных интерфейсов и только
переводят типизированные ошибки в свой протокол. Работа разбита на независимые
проверяемые коммиты и не включает массовое перемещение в `cmd/`/`internal/`.

**Технологии:** Go 1.25, `testing`, `httptest`, `gopkg.in/yaml.v2`,
VictoriaMetrics `metrics.Set`, существующие тестовый Modbus-клиент и HTTP-
транспорт Telegram; новые внешние зависимости не добавляются.

## Текущая базовая версия

Предыдущая итерация безопасного Controller завершена. На 2026-09-01 проходят:

```bash
gofmt -l .
go test -race ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Покрыты отмена и переподключение в жизненном цикле, запись во время
переподключения, изоляция снимков, контракт HTTP-записи и Bearer-аутентификации,
ошибки Telegram API и Node-RED. Эти гарантии нельзя ослаблять в следующих
задачах.

## Входные данные и принятые решения

До задачи 5 владелец оборудования должен зафиксировать точные `min`, `max` и
`step` для каждого записываемого тега:

| Тег | Адрес | Тип | Требуемые данные |
| --- | ---: | --- | --- |
| `t_otopl_ust` | 523 | `uint16` | утверждённые `min`, `max`, `step` |
| `t_floor_ust` | 524 | `uint16` | утверждённые `min`, `max`, `step` |
| `t_boiler_ust` | 525 | `uint16` | утверждённые `min`, `max`, `step` |
| `d_otopl_ust` | 526 | `uint16` | утверждённые `min`, `max`, `step` |
| `d_floor_ust` | 527 | `uint16` | утверждённые `min`, `max`, `step` |
| `d_boiler_ust` | 528 | `uint16` | утверждённые `min`, `max`, `step` |

Значения нельзя выводить из текущих показаний или придумывать. Их источник и
утверждённые числа фиксируются в `docs/REGISTER_MAP.md` в рамках задачи 5.

Для остальных неоднозначных пунктов план принимает следующие политики:

- HTTP продолжает слушать `:9101`; точки чтения остаются доступны как сейчас.
- HTTP-запись выключена по умолчанию. При `http.writeEnabled: true` непустой
  `http.writeBearerToken` обязателен; устаревший режим без аутентификации
  удаляется.
- Пустой итоговый Telegram-токен отключает Telegram. Непустой токен включает
  бота и требует непустого списка разрешённых владельцев.
- Диалоги Telegram независимы для каждой пары `(chatID, userID)`.
- План релиза предполагает Linux ARMv7 и ARM64. Перед задачей 9 нужно
  подтвердить, что ARMv6 не требуется.

## Общие ограничения

- Не менять адреса, группы, типы и имена тегов из
  `etc/modbus2prometheus.config.yaml`, кроме добавления утверждённых
  `min`/`max`/`step`.
- Не менять пути `/tags`, `/metrics`, `/api/v1/write` и имена Telegram-команд.
- Не менять существующие имена метрик `req_counter`, `err_counter` и gauge-
  метрик.
- Не менять порядок байтов и слов Modbus, интервалы опроса и лимит
  переподключений.
- Новое поведение сначала фиксируется падающим тестом; затем вносится
  минимальное изменение рабочего кода.
- Одна задача — один проверяемый коммит. Не смешивать чистку с изменением
  поведения.
- После каждой задачи запускать тесты пакета; после каждого этапа — полный
  набор итоговых проверок.
- Любая ошибка внешнего ввода или транспорта возвращается как `error`; только
  `main` определяет политику завершения процесса.

## Планируемая структура файлов

| Путь | Действие | Ответственность |
| --- | --- | --- |
| `controller/operation.go` | создать | Строгие `Operation` и `ValueKind`. |
| `controller/operation_test.go` | создать | Регрессионные тесты грамматики операций. |
| `controller/value.go` | создать позже | Типизированное кешированное значение Modbus. |
| `controller/value_test.go` | создать позже | Тесты преобразования, строк и JSON-представления. |
| `controller/validation.go` | создать | Ограничения записи и типизированные ошибки валидации. |
| `controller/validation_test.go` | создать | Тесты границ, целых значений и шага. |
| `controller/controller.go` | изменить | Типизированные операции/значения, готовность и метрики. |
| `controller/controller_test.go` | изменить | Тесты float32, валидации, готовности и метрик. |
| `controller/tags.go` | изменить | Поля операции и ограничений. |
| `controller/helpers.go` | сократить/удалить | Перенос операций и значений в отдельные файлы. |
| `controller/http.go` | изменить | Минимальные интерфейсы сервиса и отображение типизированных ошибок. |
| `controller/http_test.go` | изменить | Отключённая/аутентифицированная запись и ответы валидации. |
| `config.go` | изменить | Строгое декодирование, закрытие файла, новые поля HTTP/диапазонов, валидация. |
| `config_test.go` | изменить | Строгий YAML и табличные тесты валидации. |
| `modbus2prometheus.go` | изменить | Опциональный Telegram, валидированная конфигурация, сборка маршрутов. |
| `modbus2prometheus_test.go` | изменить | Тесты запуска, работоспособности и готовности. |
| `handlers.go` | изменить | `/healthz` и `/readyz`. |
| `telegram/commands.go` | изменить | Сессии команд на каждый диалог. |
| `telegram/bot.go` | изменить | Состояние `(chatID,userID)` и общая проверка списка разрешённых пользователей. |
| `telegram/bot_test.go` | изменить | Параллельные диалоги и неавторизованные callbacks. |
| `telegram/commands/sust.go` | изменить | Минимальный сервис тегов и сообщения типизированных ошибок. |
| `telegram/commands/sust_test.go` | изменить | Ошибки диапазона/шага и изолированные сессии. |
| `etc/modbus2prometheus.config.yaml` | изменить | Безопасный HTTP-режим и утверждённые ограничения записи. |
| `README.md` | изменить | Миграция, конфигурация и пробы. |
| `docs/PROJECT_GUIDE.md` | изменить | Фактические контракты и состояние после задач. |
| `docs/REGISTER_MAP.md` | создать в задаче 5 | Утверждённые диапазоны оборудования и их источник. |
| `.github/workflows/release.yml` | изменить | Артефакты ARMv7 и ARM64. |

## Этап A. Безопасность домена и конфигурации

### Задача 1. Зафиксировать успешную работу с float32

**Файлы:**

- Изменить: `controller/controller_test.go`

**Интерфейсы:**

- Использует: существующие `fakeRegisterClient`, `READ_FLOAT`, `WRITE_FLOAT`.
- Создаёт: регрессионное покрытие без изменения рабочего API.

- [ ] **Шаг 1. Добавить тест успешного опроса float**

Создать `TestRunReadsFloat32` с тестовым клиентом, возвращающим `float32(21.5)`.
Запустить `Controller.Run`, дождаться первого чтения, проверить равенство
`Snapshot()[0].Value` значению `float32(21.5)`, отменить контекст и потребовать
`context.Canceled`.

- [ ] **Шаг 2. Добавить тест успешной записи float**

```go
func TestWriteTagByNameWritesFloat32(t *testing.T) {
	var gotAddress uint16
	var gotValue float32
	client := &fakeRegisterClient{
		writeFloat32Fn: func(address uint16, value float32) error {
			gotAddress, gotValue = address, value
			return nil
		},
	}
	ctrl := newWithClient(Configuration{}, client, metrics.NewSet())
	ctrl.AddTag(&Tag{Name: "float_setpoint", Address: 42, Method: WRITE_FLOAT})

	if err := ctrl.WriteTagByName("float_setpoint", 21.5); err != nil {
		t.Fatalf("WriteTagByName() error = %v", err)
	}
	if gotAddress != 42 || gotValue != 21.5 {
		t.Fatalf("WriteFloat32(%d, %v), want (42, 21.5)", gotAddress, gotValue)
	}
}
```

- [ ] **Шаг 3. Запустить характеризующие тесты**

```bash
go test ./controller -run 'TestRunReadsFloat32|TestWriteTagByNameWritesFloat32' -count=1
```

Ожидается: PASS на базовой версии. Если один из тестов падает, остановиться и
исправить регрессию до изменения доменных типов.

- [ ] **Шаг 4. Создать коммит**

```bash
git add controller/controller_test.go
git commit -m "test: cover float register operations"
```

### Задача 2. Заменить поиск подстрок строгим типом операции

**Файлы:**

- Создать: `controller/operation.go`
- Создать: `controller/operation_test.go`
- Изменить: `controller/controller.go`
- Изменить: `controller/tags.go`
- Изменить: `controller/helpers.go`
- Изменить: `modbus2prometheus.go`
- Удалить после миграции: `controller/helpers_test.go`

**Интерфейсы:**

- Создаёт:

```go
type ValueKind uint8

const (
	ValueNone ValueKind = iota
	ValueUint16
	ValueFloat32
)

type Operation struct {
	Read  ValueKind
	Write ValueKind
}

func ParseOperation(raw string) (Operation, error)
func (o Operation) Readable() bool
func (o Operation) Writable() bool
```

- [ ] **Шаг 1. Написать падающий табличный тест грамматики**

Покрыть точные допустимые значения `read_uint`, `read_float`, `write_uint`,
`write_float`, `read_uint|write_uint`, `read_float|write_float`. Отклонять
пустые и неизвестные значения, дубликаты, подстроки (`prefix_read_uint`),
конфликтующие типы чтения/записи и несовпадающие типы чтения и записи.

```go
func TestParseOperationRejectsInvalidGrammar(t *testing.T) {
	for _, raw := range []string{
		"", "read", "prefix_read_uint", "read_uint|read_uint",
		"read_uint|read_float", "write_uint|write_float",
		"read_uint|write_float",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseOperation(raw); err == nil {
				t.Fatalf("ParseOperation(%q) must fail", raw)
			}
		})
	}
}
```

- [ ] **Шаг 2. Подтвердить падение строгих тестов**

```bash
go test ./controller -run TestParseOperation -count=1
```

Ожидается: FAIL, потому что текущий поиск подстрок принимает некорректные
значения.

- [ ] **Шаг 3. Реализовать точный разбор токенов**

Использовать `strings.Split(raw, "|")`, явный `switch` по четырём разрешённым
токенам и map `seen`. Отклонять более одного типа чтения, более одного типа
записи и несовпадающие ненулевые типы чтения/записи. Не обрезать пробелы и не
нормализовать ввод молча.

- [ ] **Шаг 4. Мигрировать Tag и места вызова**

Заменить `Tag.Method uint8` на `Tag.Operation Operation`. Заменить битовые
вспомогательные функции методами `Operation` и использовать
`Operation.Read`/`Operation.Write` в опросе, gauge-метриках и коде записи.
Сохранить прежние Modbus-вызовы и адреса.

- [ ] **Шаг 5. Проверить поведение и создать коммит**

```bash
go test -race ./controller ./telegram/... ./... -count=1
git add controller modbus2prometheus.go
git commit -m "refactor: parse tag operations strictly"
```

Ожидается: все тесты PASS; строки операций из конфигурации и внешние имена не
изменились.

### Задача 3. Декодировать и валидировать конфигурацию до побочных эффектов

**Файлы:**

- Изменить: `config.go`
- Изменить: `config_test.go`
- Изменить: `modbus2prometheus.go`
- Изменить: `modbus2prometheus_test.go`

**Интерфейсы:**

- Создаёт:

```go
func ValidateConfig(config *Config, maxAttempts uint) error
func TelegramEnabled(config TelegramConfig) bool
```

- [ ] **Шаг 1. Добавить тесты строгого YAML**

Добавить случаи с неизвестным корневым полем, неизвестным полем тега и вторым
YAML-документом. Каждый вызов `NewConfig` должен вернуть ошибку.

- [ ] **Шаг 2. Добавить табличные тесты валидации**

Начать с одной валидной конфигурации только для чтения и изменять по одному полю на
случай. Требовать ошибку для пустого итогового URL устройства, нулевых или
отрицательных тайм-аутов, периода опроса и периода чтения, нулевого
`maxAttempts`,
пустого, повторного или некорректного имени метрики, повторного или
пересекающегося адреса регистра и некорректной операции.

Использовать следующий контракт имени метрики:

```go
var metricNamePattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
```

При учёте занятых адресов чтение float резервирует `address` и `address+1`;
отклонять float по адресу `65535` и любое пересечение с другим тегом.

- [ ] **Шаг 3. Подтвердить падение тестов**

```bash
go test . -run 'TestNewConfig|TestValidateConfig' -count=1
```

Ожидается: FAIL, потому что декодер разрешает неизвестные поля, а
`ValidateConfig` ещё не существует.

- [ ] **Шаг 4. Сделать декодирование строгим и закрывать файл**

В `NewConfig` добавить `defer file.Close()`, вызвать `decoder.SetStrict(true)`,
декодировать ровно один документ и потребовать `io.EOF` от второй попытки.
Возвращать обёрнутую ошибку декодирования, а ошибку закрытия присоединять к именованной
возвращаемой ошибке.

- [ ] **Шаг 5. Реализовать валидацию и опциональный Telegram**

Валидировать только после применения значений по умолчанию и переопределений
CLI. Считать Telegram включённым тогда и только тогда, когда итоговый токен не
пуст.
В выключенном режиме пропускать `initTelegram`; во включённом требовать хотя бы
одного владельца. Валидировать `telegram.nodeRedUrl` только для непустого
значения и требовать абсолютный URL со схемой `http` или `https`.

- [ ] **Шаг 6. Проверить сборку компонентов при запуске**

Добавить тест, доказывающий, что пустой Telegram-токен не вызывает фабрику
Telegram. Выделить только минимальный шов фабрики, необходимый тесту; не
запускать реального бота или Modbus-устройство.

- [ ] **Шаг 7. Запустить проверки и создать коммит**

```bash
go test -race ./... -count=1
git add config.go config_test.go modbus2prometheus.go modbus2prometheus_test.go
git commit -m "feat: validate configuration before startup"
```

### Задача 4. Выключить HTTP-запись по умолчанию и требовать аутентификацию при включении

**Файлы:**

- Изменить: `config.go`
- Изменить: `config_test.go`
- Изменить: `controller/http.go`
- Изменить: `controller/http_test.go`
- Изменить: `modbus2prometheus.go`
- Изменить: `modbus2prometheus_test.go`
- Изменить: `etc/modbus2prometheus.config.yaml`
- Изменить: `README.md`

**Интерфейсы:**

```go
type HTTPConfig struct {
	WriteEnabled     bool   `yaml:"writeEnabled"`
	WriteBearerToken string `yaml:"writeBearerToken"`
}

func initHTTPServer(ctrl *controller.Controller, conf HTTPConfig) *http.ServeMux
```

- [ ] **Шаг 1. Написать тесты миграционного контракта**

Тестировать mux целиком, а не только `BearerAuth`:

| Конфигурация | Запрос | Ожидается |
| --- | --- | --- |
| `writeEnabled: false` | POST-запись | `404` |
| включено, токен пуст | валидация конфигурации | ошибка до запуска сервера |
| включено, токен задан, заголовок отсутствует | POST-запись | `401` |
| включено, токен задан, заголовок корректен | валидная POST-запись | `204` |

- [ ] **Шаг 2. Подтвердить падение тестов**

```bash
go test . ./controller -run 'Test.*Write.*Enabled|TestBearerAuth' -count=1
```

Ожидается: FAIL, потому что маршрут сейчас регистрируется всегда, а пустой токен
включает устаревший режим без аутентификации.

- [ ] **Шаг 3. Реализовать безопасную сборку маршрутов**

Регистрировать `/api/v1/write` только при `WriteEnabled == true`. Валидация
должна отклонять включённую запись с пустым или состоящим из пробелов токеном.
Сохранить сравнение токена за константное время и существующий ответ `401`.
Удалить тест, который считает пустой токен успешной обратной совместимостью.

- [ ] **Шаг 4. Документировать миграцию**

Обновить пример:

```yaml
http:
  writeEnabled: false
  # Обязательно, когда writeEnabled равно true.
  # writeBearerToken: "replace-in-installed-config"
```

README должен сообщать, что в существующих развёртываниях с HTTP-записью перед
обновлением нужно явно задать оба поля. Никогда не коммитить настоящий токен.

- [ ] **Шаг 5. Запустить регрессионные тесты безопасности и создать коммит**

```bash
go test -race ./... -count=1
git diff --check
git add config.go config_test.go controller/http.go controller/http_test.go \
  modbus2prometheus.go modbus2prometheus_test.go \
  etc/modbus2prometheus.config.yaml README.md
git commit -m "feat: require explicit authenticated http writes"
```

### Задача 5. Добавить ограничения записи оборудования и типизированные ошибки

**Условие начала:** `docs/REGISTER_MAP.md` должен содержать утверждённые
числовые значения и источник для всех шести записываемых тегов из таблицы выше.
Остановить задачу, если эти значения недоступны.

**Файлы:**

- Создать: `docs/REGISTER_MAP.md`
- Создать: `controller/validation.go`
- Создать: `controller/validation_test.go`
- Изменить: `config.go`
- Изменить: `config_test.go`
- Изменить: `controller/tags.go`
- Изменить: `controller/controller.go`
- Изменить: `controller/controller_test.go`
- Изменить: `modbus2prometheus.go`
- Изменить: `etc/modbus2prometheus.config.yaml`

**Интерфейсы:**

```go
type WriteConstraint struct {
	Min  float64
	Max  float64
	Step float64
}

var (
	ErrInvalidValue      = errors.New("invalid tag value")
	ErrValueOutOfRange   = errors.New("tag value is out of range")
	ErrValueStepMismatch = errors.New("tag value does not match step")
)

func (c WriteConstraint) Validate(value float64, kind ValueKind) error
```

`TagConfig` использует поля `*float64` для `min`, `max`, `step`, чтобы
отсутствующее значение отличалось от допустимого нуля. Во время выполнения
`Tag` содержит
конкретный `WriteConstraint` только для операций записи.

- [ ] **Шаг 1. Написать падающие тесты валидации**

Покрыть `NaN`, обе бесконечности, дробный `uint16`, значения меньше нуля и
больше 65535, переполнение float32, настроенные границы min/max и соответствие
шагу относительно `min`. Требовать успешный `errors.Is` с точной sentinel-
ошибкой выше.

```go
func TestWriteConstraintRejectsFractionalUint16(t *testing.T) {
	c := WriteConstraint{Min: 0, Max: 100, Step: 1}
	err := c.Validate(1.5, ValueUint16)
	if !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidValue)
	}
}
```

- [ ] **Шаг 2. Подтвердить падение тестов**

```bash
go test ./controller -run 'TestWriteConstraint|TestWriteTagByNameRejects' -count=1
```

Ожидается: ошибка сборки, потому что ограничения и типизированные ошибки ещё не
существуют.

- [ ] **Шаг 3. Реализовать числовую валидацию до преобразования**

Порядок валидации: конечное значение, целевой Go-тип, настроенный диапазон,
настроенный шаг. Для шага использовать `(value-Min)/Step` и относительный
допуск `1e-6` около ближайшего целого. Преобразовывать в `uint16`/`float32`
только после успешной валидации.

- [ ] **Шаг 4. Сделать ограничения обязательными в конфигурации**

Записываемые теги требуют все три поля; теги только для чтения отклоняют любое
из них.
Требовать `min <= max`, `step > 0`, конечные значения и границы, представимые
типом операции. Передавать валидированное ограничение из `initController` в
`Tag`.

- [ ] **Шаг 5. Зафиксировать и применить карту оборудования**

`docs/REGISTER_MAP.md` должен перечислять тег, адрес, тип, утверждённые
min/max/step, исходный документ и его версию, а также дату утверждения.
Скопировать точные значения в `etc/modbus2prometheus.config.yaml`; не изменять
ни одно существующее поле тега.

- [ ] **Шаг 6. Проверить, что некорректный ввод не достигает Modbus**

Для каждой типизированной ошибки проверить `writeRegisterCalls == 0` и
`writeFloat32Calls == 0`. Добавить успешные граничные случаи, доказывающие, что
min/max достигают правильного Modbus-метода.

- [ ] **Шаг 7. Запустить проверки и создать коммит**

```bash
go test -race ./... -count=1
git diff -- etc/modbus2prometheus.config.yaml docs/REGISTER_MAP.md
git add config.go config_test.go controller modbus2prometheus.go \
  etc/modbus2prometheus.config.yaml docs/REGISTER_MAP.md
git commit -m "feat: enforce equipment write constraints"
```

## Этап B. Границы адаптеров и изоляция Telegram

### Задача 6. Сделать HTTP и Telegram тонкими адаптерами единого сервиса записи

**Файлы:**

- Изменить: `controller/http.go`
- Изменить: `controller/http_test.go`
- Изменить: `telegram/commands/sust.go`
- Изменить: `telegram/commands/sust_test.go`
- Изменить: `modbus2prometheus.go`

**Интерфейсы:**

Каждый адаптер определяет минимальные используемые им интерфейсы. В
`controller/http.go` они остаются в пакете `controller`:

```go
type tagJSONReader interface {
	Json() ([]byte, error)
}

type tagWriter interface {
	WriteTagByName(name string, value float64) error
}
```

В `telegram/commands/sust.go` интерфейс сервиса использует импортированный DTO
Controller:

```go
type tagService interface {
	Snapshot() []controller.TagSnapshot
	WriteTagByName(name string, value float64) error
}
```

Controller остаётся конкретной реализацией. Валидация и преобразование остаются
внутри `Controller.WriteTagByName`; адаптеры не дублируют доменные проверки.

- [ ] **Шаг 1. Добавить тесты отображения ошибок адаптерами**

Использовать тестовые реализации записи, возвращающие каждую sentinel-ошибку.
HTTP отображает ошибки
некорректного значения, диапазона и шага в `422` со стабильными JSON-
сообщениями. Telegram отправляет понятный отказ и не сообщает об успехе.
Существующие отображения `404`, `403` и `502` сохраняются.

- [ ] **Шаг 2. Подтвердить падение тестов**

```bash
go test ./controller ./telegram/commands -run 'Test.*Write.*Error|Test.*Setpoint.*Error' -count=1
```

Ожидается: FAIL, потому что обработчики принимают конкретный Controller и не
отображают новые типизированные ошибки.

- [ ] **Шаг 3. Инвертировать зависимости адаптеров**

Изменить конструкторы обработчиков и команд так, чтобы они принимали локальные
интерфейсы. Оставить все доменные решения в Controller. Возвращать стабильные
протокольные сообщения, не раскрывая внутренние ошибки в HTTP-ответах.

- [ ] **Шаг 4. Проверить одинаковые правила в обоих адаптерах**

Использовать одну таблицу некорректных значений, значений ниже min, выше max и
не соответствующих шагу в тестах HTTP и Telegram. Проверить, что тестовая
реализация записи
вызывается один раз и определяет доменный результат; отдельно сохранить тесты
Controller, доказывающие, что некорректные значения не вызывают Modbus.

- [ ] **Шаг 5. Запустить проверки и создать коммит**

```bash
go test -race ./controller ./telegram/... ./... -count=1
git add controller/http.go controller/http_test.go telegram/commands \
  modbus2prometheus.go
git commit -m "refactor: share write rules across adapters"
```

### Задача 7. Изолировать состояние Telegram по чату и пользователю

**Файлы:**

- Изменить: `telegram/commands.go`
- Изменить: `telegram/bot.go`
- Изменить: `telegram/bot_test.go`
- Изменить: `telegram/commands/sust.go`
- Изменить: `telegram/commands/sust_test.go`
- Изменить: `telegram/commands/sens.go`
- Изменить: `modbus2prometheus.go`

**Интерфейсы:**

```go
type conversationKey struct {
	ChatID int64
	UserID int64
}

type conversationState struct {
	command     ICommand
	lastUpdated time.Time
}

type ICommand interface {
	Command() string
	Description() string
	NewSession() ICommand
	Reply() string
	Action(bot *tgbotapi.BotAPI, update tgbotapi.Update) bool
	Callback(bot *tgbotapi.BotAPI, update tgbotapi.Update) (bool, error)
}
```

Каждая интерактивная команда получает новый экземпляр сессии. `SimpleCommand`
может возвращать себя, потому что у неё нет изменяемого состояния диалога;
`UstCommand` возвращает новый экземпляр только с тем же сервисом тегов.

- [ ] **Шаг 1. Выделить идентификатор update и тестовые швы обработчика**

Добавить вспомогательные функции, возвращающие `(conversationKey, ok)` для Message и
CallbackQuery. Идентификатор callback использует
`CallbackQuery.Message.Chat.ID` и `CallbackQuery.From.ID`; некорректные
callbacks игнорируются без panic. Выделить `BotState.handleUpdate(update) error`
из горутины опроса.

- [ ] **Шаг 2. Написать тест неавторизованного callback**

Создать callback от пользователя, отсутствующего в `Owners`, при активном
диалоге другого владельца. Потребовать ноль вызовов callback команды и
неизменившееся состояние.

- [ ] **Шаг 3. Написать тест параллельных диалогов**

Запустить `/sust` для двух разрешённых пар `(chatID,userID)`, выбрать разные
теги и отправить разные значения. Проверить, что каждая запись тестового сервиса
использует тег, выбранный той же парой. Тест должен падать с текущим глобальным
состоянием `currentCommand`/`currentTagName`.

- [ ] **Шаг 4. Реализовать сессии по ключу и очистку по тайм-ауту**

Заменить единственные `currentCommand` и отметку времени на
`map[conversationKey]conversationState`. Применять пятиминутный тайм-аут к
каждой записи и удалять завершённые или ошибочные записи. Выполнять общую
проверку владельца до обработки Message и CallbackQuery.

- [ ] **Шаг 5. Удалить неиспользуемое глобальное состояние команд**

Удалить `UstCommand.curChatId` и `SensorsCommand.currentVar`. Оставить
`UstCommand.currentTagName` только в экземпляре сессии конкретного диалога.

- [ ] **Шаг 6. Запустить race-тесты и создать коммит**

```bash
go test -race ./telegram/... -count=1
go test -race ./... -count=1
git add telegram modbus2prometheus.go
git commit -m "refactor: isolate telegram conversations"
```

Ожидается: одновременно работающие владельцы не влияют на выбор тегов друг
друга, а неавторизованные callbacks не могут вызывать команды.

## Этап C. Эксплуатация, релизы и чистка

### Задача 8. Добавить пробы работоспособности, готовности и операционные метрики Controller

**Файлы:**

- Изменить: `controller/controller.go`
- Изменить: `controller/controller_test.go`
- Изменить: `handlers.go`
- Изменить: `modbus2prometheus.go`
- Изменить: `modbus2prometheus_test.go`
- Изменить: `README.md`

**Интерфейсы:**

```go
func (c *Controller) Ready() bool
func HealthHandler() http.HandlerFunc
func ReadyHandler(source interface{ Ready() bool }) http.HandlerFunc
```

Новые метрики в наборе Controller:

```text
modbus_reconnect_attempts_total
modbus_write_requests_total
modbus_write_errors_total
```

- [ ] **Шаг 1. Написать тесты переходов готовности**

Требовать false до первого успешного чтения, true после успешного чтения,
false после ошибки чтения и закрытия клиента и снова true после
переподключения и успешного чтения.

- [ ] **Шаг 2. Написать тесты HTTP-проб**

`GET /healthz` всегда возвращает `200` и `ok\n`, пока сервер работает.
`GET /readyz` возвращает `503` и `not ready\n` для false и `200` и `ready\n`
для true. Остальные методы возвращают `405` с `Allow: GET`.

- [ ] **Шаг 3. Написать тесты метрик**

Использовать экземпляр `metrics.Set`, вызвать одно неуспешное переподключение, одну
успешную и одну неуспешную запись, вызвать `set.WritePrometheus(&buffer)` и
проверить точные значения счётчиков. Не зависеть от глобального реестра в
модульных тестах.

- [ ] **Шаг 4. Реализовать состояние и счётчики под существующей синхронизацией**

Готовность относится к состоянию Controller и изменяется в тех же критических
секциях, что и переходы клиента/чтения. Увеличивать счётчик переподключений перед
каждой попыткой повторного открытия, счётчик запросов записи — до
валидации, открытия и записи, а счётчик ошибок записи — для каждой ошибки пути записи
после выбора известного записываемого тега.

- [ ] **Шаг 5. Зарегистрировать и документировать пробы**

Добавить `/healthz` и `/readyz` в существующий mux. Документировать, что
готовность требует хотя бы одного успешного чтения Modbus и становится false
после ошибки чтения; не описывать её как общую проверку зависимостей.

- [ ] **Шаг 6. Запустить проверки и создать коммит**

```bash
go test -race ./... -count=1
git add controller/controller.go controller/controller_test.go handlers.go \
  modbus2prometheus.go modbus2prometheus_test.go README.md
git commit -m "feat: expose controller health and operation metrics"
```

### Задача 9. Публиковать явные артефакты релиза ARMv7 и ARM64

**Условие начала:** подтвердить, что ARMv6 не требуется. Если он требуется,
добавить его третьей явной целью матрицы; не заменять ARMv7 молча.

**Файлы:**

- Изменить: `.github/workflows/release.yml`
- Изменить: `README.md`
- Изменить: `docs/PROJECT_GUIDE.md`

**Создаёт:**

- `modbus2prometheus-linux-armv7.tar.gz`, собранный с
  `GOOS=linux GOARCH=arm GOARM=7`.
- `modbus2prometheus-linux-arm64.tar.gz`, собранный с
  `GOOS=linux GOARCH=arm64`.

- [ ] **Шаг 1. Локально проверить обе кросс-сборки**

```bash
GOOS=linux GOARCH=arm GOARM=7 go build -o /tmp/modbus2prometheus-linux-armv7 .
GOOS=linux GOARCH=arm64 go build -o /tmp/modbus2prometheus-linux-arm64 .
```

Ожидается: обе команды завершаются с кодом 0. Временные бинарники не
добавляются в Git.

- [ ] **Шаг 2. Заменить неявную ARM-сборку явной матрицей**

Использовать записи матрицы с `artifact`, `goarch` и опциональным `goarm`;
собирать, архивировать и загружать артефакт с уникальным именем для каждой цели.
Сохранить проверяющую задачу с `go-version-file: go.mod`.

- [ ] **Шаг 3. Документировать поддерживаемые цели и создать коммит**

```bash
git diff --check
git add .github/workflows/release.yml README.md docs/PROJECT_GUIDE.md
git commit -m "ci: publish armv7 and arm64 releases"
```

### Задача 10. Удалить проверки типов во время выполнения и неиспользуемое внутреннее состояние

**Файлы:**

- Создать: `controller/value.go`
- Создать: `controller/value_test.go`
- Изменить: `controller/tags.go`
- Изменить: `controller/controller.go`
- Изменить: `controller/helpers.go`
- Изменить: `controller/json.go`
- Изменить: `controller/http.go`
- Изменить: `controller/*_test.go`
- Изменить: `modbus2prometheus.go`
- Изменить: `telegram/commands/*.go`

**Интерфейсы:**

```go
type TagValue struct {
	Kind    ValueKind
	Uint16  uint16
	Float32 float32
	Valid   bool
}

func Uint16Value(value uint16) TagValue
func Float32Value(value float32) TagValue
func (v TagValue) Float64() float64
func (v TagValue) Interface() any
func (v TagValue) String() string
```

Только конструктор, соответствующий `Kind`, создаёт валидное значение.
`Interface()` существует только для сохранения чисел в JSON `/tags`; адаптеры
не проверяют поля объединения.

- [ ] **Шаг 1. Написать тесты значений**

Покрыть нулевые, но валидные значения, отсутствие значения, форматирование
float с двумя дробными знаками, форматирование uint и преобразование в
примитивы для JSON.

- [ ] **Шаг 2. Мигрировать кешированные значения без изменения вывода**

Заменить `Tag.LastValue interface{}` и `TagSnapshot.Value any` на `TagValue`.
Ветки чтения создают правильное значение; gauge-метрики используют `Float64`,
Telegram — `String`, а `Json()` — `Interface()`. Существующая эталонная форма
`/tags` и значения метрик должны остаться неизменными.

- [ ] **Шаг 3. Переименовать и удалить неиспользуемый код**

Переименовать `TagsHahdler` в `TagsHandler` и обновить единственное место
вызова. Удалить неиспользуемые поле/тип `logger` и комментарии с оставленным
кодом. Не переименовывать внешние `ust`, `sust`, пути и имена метрик.

- [ ] **Шаг 4. Запустить сфокусированные тесты совместимости**

```bash
go test ./controller -run 'TestSnapshot|Test.*Json|Test.*Float|Test.*Uint' -count=1
go test ./telegram/... -count=1
```

Ожидается: числовые типы JSON, форматированные значения Telegram и gauge-метрики
соответствуют контракту до задачи.

- [ ] **Шаг 5. Запустить полные race-тесты и создать коммит**

```bash
go test -race ./... -count=1
git add controller modbus2prometheus.go telegram
git commit -m "refactor: use typed tag values"
```

### Задача 11. Синхронизировать GUIDE, README и примеры развёртывания

**Файлы:**

- Изменить: `README.md`
- Изменить: `docs/PROJECT_GUIDE.md`
- Изменить: `docs/REFACTORING_PLAN.md`
- Проверить: `etc/modbus2prometheus.config.yaml`
- Проверить: `docker/docker-compose.yml`
- Проверить: `docker/nginx.conf`
- Проверить: `etc/systemd/system/*.service`
- Проверить: `.github/workflows/*.yml`

- [ ] **Шаг 1. Обновить документацию текущего состояния**

Перенести каждый завершённый пункт в `PROJECT_GUIDE.md` из ограничений и
запланированных этапов в реализованный контракт. Явно сохранить нерешённые
условия начала. Отмечать флажок плана выполненным только при наличии
соответствующего коммита и подтверждения тестами.

- [ ] **Шаг 2. Проверить каждый документированный путь и поле конфигурации**

```bash
test -f etc/modbus2prometheus.config.yaml
test -f etc/systemd/system/modbus2prometheus.service
test -f etc/systemd/system/vmagent-scraper.service
test -f etc/vmagent.scrape.config.yaml
rg -n 'writeEnabled|writeBearerToken|min:|max:|step:' \
  config.go README.md docs etc/modbus2prometheus.config.yaml
```

Ожидается: все пути существуют, а каждое документированное поле имеет
определение в коде и пример. В отслеживаемых файлах нет настоящих токенов или
учётных данных.

- [ ] **Шаг 3. Выполнить финальное ревью документации**

Проверить, что Docker описан с сетью хоста, HTTP-запись нигде не описана
как неаутентифицированная, включение Telegram соответствует условию итогового
токена, семантика проб соответствует коду, а имена артефактов релиза —
workflow-файлу.

- [ ] **Шаг 4. Запустить итоговый набор проверок**

```bash
gofmt -l .
go test -race ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Ожидается:

- `gofmt -l .` ничего не выводит;
- все тесты PASS без сообщений детектора гонок;
- vet, build и diff check завершаются с кодом 0;
- адреса регистров, имена тегов, группы и существующие имена метрик не изменены;
- выключенная HTTP-запись недоступна;
- включённая HTTP-запись требует валидный Bearer-токен;
- каждая принятая запись соответствует целевому типу и утверждённому диапазону
  оборудования;
- одновременные владельцы Telegram имеют изолированные диалоги;
- пробы и новые счётчики покрыты детерминированными тестами.

- [ ] **Шаг 5. Создать финальный коммит документации**

```bash
git add README.md docs etc .github/workflows
git commit -m "docs: close hardening plan"
```

## Критерии завершения

План завершён только тогда, когда все флажки задач подтверждены в Git, а
финальный набор проверок проходит на чистом рабочем дереве. Аппаратный
smoke-тест — отдельный шаг развёртывания: прочитать все настроенные теги,
записать только явно
утверждённое безопасное значение, проверить Telegram из двух разрешённых
аккаунтов и `/readyz` во время контролируемого отключения и переподключения
Modbus. Зафиксировать устройство, прошивку, время и результат в журнале
развёртывания; не объявлять smoke-тест пройденным только на основании модульных
тестов.
