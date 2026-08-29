# Safe Controller Refactoring Implementation Plan

**Goal:** устранить опасные ошибки жизненного цикла и создать тестируемую
границу вокруг Modbus, не меняя карту регистров, HTTP/Telegram contracts и
имена метрик.

**Architecture:** Controller получает минимальный интерфейс Modbus-клиента и
собственный metrics set. Цикл polling становится `Run(context.Context) error`:
он возвращает причину остановки вызывающему коду, а политикой exit/restart
управляет `main`. Первые исправления делаются в существующей структуре пакетов;
перемещение в `internal/` в этот план не входит.

**Tech stack:** Go 1.25 module semantics, `testing`, `context`,
`net/http/httptest`, VictoriaMetrics metrics set, существующий
`simonvetter/modbus v1.6.0`.

## Global Constraints

- Не изменять адреса, типы, группы и имена тегов из
  `etc/modbus2prometheus.config.yaml`.
- Не изменять существующие URL paths, JSON field names и Telegram command names.
- Не добавлять новую внешнюю зависимость для mocks или assertions.
- Одна задача — один independently reviewable commit.
- Каждый bugfix проходит red-green: failing regression test до исправления.
- Все ожидаемые внешние ошибки возвращаются как `error`; библиотечные пакеты не
  вызывают `os.Exit`, `log.Fatal` или panic.
- Этот план не вводит постоянную схему HTTP-auth и диапазоны уставок: для них
  нужны решения из раздела 14 `PROJECT_GUIDE.md`.

## File Structure for This Plan

| Путь | Действие | Ответственность после этапа |
| --- | --- | --- |
| `controller/client.go` | создать | Минимальный интерфейс Modbus и production factory. |
| `controller/client_test.go` | создать | Compile-time проверка fake client. |
| `controller/controller.go` | изменить | Dependency injection, cancelable polling, без process exit. |
| `controller/controller_test.go` | создать | Polling, reconnect, cancel, write tests. |
| `controller/tags.go` | изменить | Синхронизированный доступ к последнему значению. |
| `controller/http_test.go` | создать | Контракт методов и ошибок HTTP write. |
| `telegram/commands/sens.go` | изменить | Безопасная обработка GET и pressure. |
| `telegram/commands/sens_test.go` | создать | Форматирование и network errors. |
| `telegram/commands/sust.go` | изменить | Разбор значения до записи. |
| `telegram/commands/sust_test.go` | создать | Invalid value never reaches writer. |
| `modbus2prometheus.go` | изменить | Context, signals, keyed config, HTTP server. |
| `README.md` | изменить | Только подтверждённые команды и согласованный порт. |
| `docker/*` | проверить | Container paths, nginx routes и доступность HTTP. |
| `.github/workflows/*.yml` | изменить | CI/release toolchain и проверки проекта. |

## Task 1: Исправить чистые Telegram/Node-RED ошибки

**Files:**

- Modify: `telegram/commands/sens.go`
- Create: `telegram/commands/sens_test.go`

**Interfaces:**

- Consumes: существующие `SensorJson`, `SensorData`, `parseData`.
- Produces: `fetchSensors(ctx context.Context) ([]SensorJson, error)` и
  корректное форматирование `Pressure`.

- [ ] **Step 1: написать падающий тест pressure**

```go
func TestParseDataDetailsUsesPressure(t *testing.T) {
	got := parseData("details", []SensorJson{{
		Name: "room",
		Data: SensorData{Humidity: 45.2, Pressure: 748.1},
	}})
	if !strings.Contains(got, "P:748.1 mmR") {
		t.Fatalf("details must contain pressure, got %q", got)
	}
}
```

- [ ] **Step 2: подтвердить регрессию**

Run:

```bash
go test ./telegram/commands -run TestParseDataDetailsUsesPressure -count=1
```

Expected: FAIL, потому что текущий ответ содержит `45.2` вместо `748.1`.

- [ ] **Step 3: заменить поле в `parseData`**

В pressure-строке использовать `sensor.Data.Pressure`. Удалить бессмысленную
проверку `if data == nil` внутри `range`: nil slice и так даёт ноль итераций.

- [ ] **Step 4: проверить pressure fix**

Run: та же команда.

Expected: PASS.

- [ ] **Step 5: написать тест ошибки Node-RED**

Создать `httptest.Server`, handler которого закрывает соединение через
`http.Hijacker`, либо подставить transport, возвращающий sentinel error.
Предпочтителен маленький transport без сети:

```go
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestFetchSensorsReturnsRequestError(t *testing.T) {
	want := errors.New("node-red unavailable")
	cmd := NewSensorsCommand("http://node-red/current_th")
	cmd.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, want
	})

	_, err := cmd.fetchSensors(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}
```

Expected before implementation: build failure because `fetchSensors` does not
exist.

- [ ] **Step 6: реализовать `fetchSensors`**

Контракт метода:

```go
func (s *SensorsCommand) fetchSensors(ctx context.Context) ([]SensorJson, error)
```

Он должен создать request через `http.NewRequestWithContext`, выполнить
`s.client.Do`, немедленно вернуть request error, закрыть body только при
ненулевом response, требовать status `200..299` и декодировать JSON. Callback
только переводит результат в Telegram-текст.

- [ ] **Step 7: проверить пакет и commit**

```bash
go test ./telegram/commands -count=1
git add telegram/commands/sens.go telegram/commands/sens_test.go
git commit -m "fix: handle sensor response errors safely"
```

Expected: package PASS; один commit содержит только sensor fix.

## Task 2: Запретить запись некорректной Telegram-уставки

**Files:**

- Modify: `telegram/commands/sust.go`
- Create: `telegram/commands/sust_test.go`

**Interfaces:**

- Consumes: текст Telegram-сообщения.
- Produces: pure helper `parseSetpoint(text string) (float64, error)`.

- [ ] **Step 1: написать table test парсера**

```go
func TestParseSetpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{name: "integer", input: "42", want: 42},
		{name: "decimal", input: "42.5", want: 42.5},
		{name: "text", input: "wrong", wantErr: true},
		{name: "nan", input: "NaN", wantErr: true},
		{name: "positive infinity", input: "+Inf", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSetpoint(tt.input)
			if (err != nil) != tt.wantErr || (!tt.wantErr && got != tt.want) {
				t.Fatalf("parseSetpoint(%q) = %v, %v", tt.input, got, err)
			}
		})
	}
}
```

- [ ] **Step 2: подтвердить отсутствие helper**

```bash
go test ./telegram/commands -run TestParseSetpoint -count=1
```

Expected: build failure, `undefined: parseSetpoint`.

- [ ] **Step 3: реализовать pure helper**

Использовать `strconv.ParseFloat(text, 32)` и `math.IsNaN`/`math.IsInf`.
Возвращать error до любого вызова Controller.

- [ ] **Step 4: изменить `UstCommand.Action`**

При ошибке `parseSetpoint` отправить сообщение об ошибке и `return true` до
`WriteTag`. Не переиспользовать переменную parse error для результата записи.

- [ ] **Step 5: проверить и commit**

```bash
go test ./telegram/commands -count=1
git add telegram/commands/sust.go telegram/commands/sust_test.go
git commit -m "fix: reject invalid telegram setpoints"
```

Expected: package PASS; invalid/NaN/Inf inputs не доходят до write branch.

## Task 3: Ввести интерфейс Modbus-клиента

**Files:**

- Create: `controller/client.go`
- Create: `controller/client_test.go`
- Modify: `controller/controller.go`

**Interfaces:**

- Consumes: точные signatures `simonvetter/modbus v1.6.0`.
- Produces:

```go
type registerClient interface {
	Open() error
	Close() error
	SetUnitId(uint8) error
	ReadRegister(uint16, modbus.RegType) (uint16, error)
	ReadFloat32(uint16, modbus.RegType) (float32, error)
	WriteRegister(uint16, uint16) error
	WriteFloat32(uint16, float32) error
}
```

- [ ] **Step 1: создать compile-time fake**

`fakeRegisterClient` должен иметь function fields для семи методов и счётчики
вызовов. Добавить:

```go
var _ registerClient = (*modbus.ModbusClient)(nil)
var _ registerClient = (*fakeRegisterClient)(nil)
```

Expected до создания interface: build failure.

- [ ] **Step 2: создать `registerClient` и production factory**

`newRegisterClient(conf Configuration) (registerClient, error)` создаёт
`modbus.ModbusClient`, устанавливает unit ID, открывает соединение и при ошибке
после создания закрывает уже открытый ресурс.

- [ ] **Step 3: добавить constructor injection**

Оставить public `New(conf *Configuration)` как production API. Добавить
неэкспортируемый:

```go
func newWithClient(conf Configuration, client registerClient, set *metrics.Set) *Controller
```

Controller хранит `registerClient`, а counters/gauges создаются в переданном
`metrics.Set`. `New` создаёт и регистрирует новый set; тесты передают свой set.

- [ ] **Step 4: проверить совместимость**

```bash
go test ./controller -count=1
go test ./... -count=1
```

Expected: PASS; production call sites по-прежнему используют `controller.New`.

- [ ] **Step 5: commit**

```bash
git add controller/client.go controller/client_test.go controller/controller.go
git commit -m "refactor: inject modbus client into controller"
```

## Task 4: Сделать polling cancelable и убрать process exit

**Files:**

- Modify: `controller/controller.go`
- Create/Modify: `controller/controller_test.go`

**Interfaces:**

- Consumes: `context.Context`, injected `registerClient`.
- Produces:

```go
func (c *Controller) Run(ctx context.Context) error
func (c *Controller) Close() error
```

- [ ] **Step 1: тест отмены**

Fake client возвращает одно значение без задержки. Запустить `Run` в goroutine,
дождаться первого read через channel, вызвать `cancel`, потребовать возврат
`context.Canceled` и ровно один `Close`.

```go
select {
case err := <-done:
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
case <-time.After(time.Second):
	t.Fatal("Run did not stop after cancel")
}
```

Expected на текущем коде: test не компилируется, потому что `Run` отсутствует.

- [ ] **Step 2: реализовать context-aware waits**

Заменить каждый `time.Sleep` helper-функцией:

```go
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
```

`Run` не меняет флаг `exit`, не вызывает `os.Exit`, закрывает клиент через один
defer и возвращает error.

- [ ] **Step 3: тест лимита reconnect**

Настроить fake так, чтобы read и последующие Open всегда возвращали sentinel
errors, `MaxAttempts=3`, durations равнялись нулю. Потребовать ровно три
reconnect attempts и error, wrapping последний Open error.

- [ ] **Step 4: исправить счётчик attempts**

Увеличивать attempts на каждой неуспешной попытке reopen. Обнулять только после
успешного чтения. Не сбрасывать error через process restart внутри Controller.

- [ ] **Step 5: race verification и commit**

```bash
go test -race ./controller -count=1
git add controller/controller.go controller/controller_test.go
git commit -m "refactor: make controller lifecycle context driven"
```

Expected: PASS без hang и без process exit.

## Task 5: Сделать доступ к значениям согласованным

**Files:**

- Modify: `controller/tags.go`
- Modify: `controller/controller.go`
- Modify: `controller/helpers.go`
- Modify: `controller/json.go`
- Modify: `modbus2prometheus.go`
- Modify: `controller/controller_test.go`

**Interfaces:**

- Produces immutable DTO:

```go
type TagSnapshot struct {
	Name        string
	DisplayName string
	Group       string
	Address     uint16
	Value       any
	Writable    bool
}

func (c *Controller) Snapshot() []TagSnapshot
```

- [ ] **Step 1: тест snapshot isolation**

Добавить теги, получить snapshot, изменить возвращённый slice и убедиться, что
повторный `Snapshot()` не изменился. Одновременно обновлять fake reads и читать
snapshot в цикле.

- [ ] **Step 2: реализовать snapshot под `RLock`**

Не возвращать `c.tags` наружу. HTTP JSON и Telegram list commands читают только
snapshot. Поиск writable tag возвращает внутренний объект только сервису
записи, не presentation adapters.

- [ ] **Step 3: удалить `exit bool` и доступ к LastValue вне lock**

Lifecycle уже управляется context. `ValToStr` должен принимать snapshot/value,
а не читать mutable `*Tag` из Telegram.

- [ ] **Step 4: полная race-проверка и commit**

```bash
go test -race ./... -count=1
git add controller modbus2prometheus.go
git commit -m "refactor: expose immutable tag snapshots"
```

Expected: PASS без race reports.

## Task 6: Уточнить HTTP contract и server lifecycle

**Files:**

- Create: `controller/http_test.go`
- Modify: `controller/http.go`
- Modify: `modbus2prometheus.go`

**Interfaces:**

- Existing paths remain `/tags`, `/metrics`, `/api/v1/write`.
- Write accepts only POST and JSON `WriteTag`.
- Main owns `http.Server`, signals and shutdown timeout.

- [ ] **Step 1: table tests методов**

Через `httptest.NewRecorder` проверить:

| Request | Expected |
| --- | --- |
| `GET /api/v1/write` | `405`, header `Allow: POST` |
| malformed POST JSON | `400` |
| unknown tag | `404` |
| read-only tag | `403` |
| client write error | `502` |
| valid write | `204` |

Сначала зафиксировать хотя бы `GET` и malformed JSON как failing tests.

- [ ] **Step 2: реализовать единый JSON error writer**

Ответы имеют `Content-Type: application/json` и форму:

```json
{"error":"invalid request body"}
```

Decoder ограничивает body через `http.MaxBytesReader` и запрещает неизвестные
поля через `DisallowUnknownFields`.

- [ ] **Step 3: заменить `http.ListenAndServe` на `http.Server`**

Задать `ReadHeaderTimeout=5s`, `ReadTimeout=10s`, `WriteTimeout=30s`,
`IdleTimeout=60s`. `main` создаёт context для `SIGINT`/`SIGTERM`, отменяет
Controller и вызывает `Shutdown` с timeout 10 секунд.

- [ ] **Step 4: verification и commit**

```bash
go test -race ./... -count=1
go vet ./...
go build ./...
git add controller/http.go controller/http_test.go modbus2prometheus.go
git commit -m "refactor: define http and shutdown contracts"
```

Expected: все три команды exit 0.

## Task 7: Синхронизировать документацию и deployment examples

**Files:**

- Modify: `README.md`
- Modify: `etc/vmagent.scrape.config.yaml`
- Review: `etc/systemd/system/modbus2prometheus.service`
- Review: `docker/docker-compose.yml`, `docker/nginx.conf`
- Modify: `.github/workflows/go.yml`, `.github/workflows/release.yml`
- Modify: `docs/PROJECT_GUIDE.md`

**Interfaces:** единый default HTTP address/port и существующие systemd paths.

- [ ] **Step 1: проверить единый порт**

Проверить, что default приложения, vmagent target, nginx upstream и systemd
используют порт `9101`. Не менять порт без отдельного migration note.

- [ ] **Step 2: исправить installation paths**

README должен ссылаться на реально существующие:

```text
etc/modbus2prometheus.config.yaml
etc/systemd/system/modbus2prometheus.service
etc/vmagent.scrape.config.yaml
etc/systemd/system/vmagent-scraper.service
```

Не публиковать настоящий Telegram token или credentials vmagent.

- [ ] **Step 3: документировать optional/required Telegram**

Описание должно соответствовать реализации после Task 6. Если Telegram всё
ещё обязателен, это указывается как runtime requirement; если запуск стал
опциональным, приводится точное условие включения из кода.

- [ ] **Step 4: выполнить команды из README**

Минимальная проверка без оборудования:

```bash
go test -race ./... -count=1
go vet ./...
go build ./...
```

Expected: exit 0. Запуск против реального Modbus и Telegram оформляется как
hardware smoke test и не объявляется проверенным локально.

- [ ] **Step 5: финальный diff review и commit**

```bash
git diff --check
git diff -- README.md etc docs
git add README.md etc/vmagent.scrape.config.yaml docs .github/workflows
git commit -m "docs: align deployment guide with runtime defaults"
```

Проверить отсутствие изменений register addresses, tag names, credentials и
systemd `ExecStart` path.

## Completion Gate

План первой итерации завершён только при одновременном выполнении:

```bash
gofmt -l .
go test -race ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected:

- `gofmt -l .` не выводит файлов;
- тесты имеют реальные test cases во всех затронутых пакетах и проходят;
- race detector не сообщает гонок;
- vet/build/diff check завершаются с exit code 0;
- ручной diff review не показывает случайных изменений карты Modbus;
- HTTP write остаётся закрыт от неавторизованной сети до отдельного плана auth;
- некорректное значение Telegram никогда не вызывает Modbus write.

После этого отдельно составляются планы для config/domain validation,
HTTP-auth и per-chat Telegram state. Они не объединяются с текущим этапом,
потому что имеют независимые security и migration decisions.
