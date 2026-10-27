# nil: аудит двух форм и варианты упрощения

Дата: 2026-10-27. Ветка: `oneway-nil`. Код не менялся; это проектное решение.

## Короткий вывод

Две формы существуют не как два равноправных способа, а как обход одного
ограничения: `var` не принимает тип, поэтому в инициализаторе нет expected type.
`applyNil` подставляет тип из контекста во всех остальных местах, `nil[T]` нужен
только там, где контекста нет.

Оставить ровно одну форму можно:

- **A. только `nil[T]`** — S, interop не ломается, но каждый nil становится
  многословным;
- **B. только bare `nil`** — S для компилятора, но язык теряет возможность
  объявить nil-переменную указателя, функции или интерфейса; в таком виде не
  готово к 1.0;
- **C. bare `nil` + `var p: *T = nil`** — M, даёт один канонический nil и
  сохраняет выразительность.

Рекомендация: **C для 1.0**, если typed `var` успевает в релиз. Если 1.0
заморожен по фичам — **A**, но не B.

## 1. Как это работает сейчас

### Парсер и AST

`internal/parser/parser.go:1323` `parseNilLit` читает `nil` и, если следом стоит
`[`, разбирает тип. Результат — `ast.NilLit{Type: T}`; у bare `nil` поле `Type`
равно `nil`.

`internal/ast/ast.go:400`:

```go
type NilLit struct {
    Span
    Type Type
}
```

Формы не различаются на уровне AST: `nil` и `nil[T]` — один узел, отличается
только заполненность `Type`.

### Sema: `applyNil` и `resolveType`

`internal/sema/expr.go:496`:

```go
func (c *checker) applyNil(e ast.Expr, want ast.Type) {
    if n, ok := e.(*ast.NilLit); ok && n.Type == nil && want != nil {
        n.Type = want
    }
}
```

Это не проверка, а подстановка expected type перед `resolveExpr`. Если тип уже
задан явно (`nil[T]`), `applyNil` его не трогает.

`resolveExpr` для `NilLit` (`internal/sema/expr.go:82`):

1. если `Type == nil` — ошибка `cannot infer type of nil; use nil[T] or an expected type`;
2. иначе `p.resolveType(...)` разворачивает имя/пакет/указатель/срез/map/функцию
   (в том числе Go-типы из импортов);
3. `nilable(t)` проверяет, что тип допускает nil:
   - `*T`, `[]T`, `map[K]V`, `fn(...)`;
   - `any`, `error`;
   - объявленный `interface` или `enum`;
   - alias — рекурсивно по цели;
   - Go-тип, у которого underlying — pointer/slice/map/signature/interface.
4. иначе — `nil is not valid for T`.

Для bare `nil` шаги те же: `applyNil` заполнил `Type`, дальше `resolveExpr`
разрешает и валидирует его как обычный `nil[T]`.

`resolveType` (`internal/sema/resolve.go:174`) рекурсивно разрешает тип и
помечает импорты; для Go-типов возвращает `GoType`, который позже связывается с
`go/types` через `mapGoType`/`goTypeOf`.

### Где bare `nil` получает тип

`applyNil` вызывается в девяти местах:

| Контекст | Место |
|---|---|
| `a, b = ...` при совпадении арности | `internal/sema/check.go:255` |
| `return ...`, включая неявный `error` | `internal/sema/check.go:298` |
| `x == nil` / `x != nil` (обе стороны) | `internal/sema/expr.go:57,64` |
| аргумент вызова, включая Go-функции | `internal/sema/expr.go:356` |
| ключ и значение map-литерала | `internal/sema/expr.go:557,558` |
| элемент slice/array-литерала | `internal/sema/expr.go:572` |
| поле struct-литерала | `internal/sema/expr.go:610` |
| результат arrow-функции | `internal/sema/expr.go:672` |

Для аргументов Go-функций expected type приходит из `goCallInfo`/`mapGoType`;
поэтому `f(nil)` работает и для `error`, и для `tea.Cmd`, и для
`context.Context`. Вариадический параметр разворачивается до элемента среза.

### Где нужен `nil[T]`

- локальный `var p = nil`;
- package-level `var p = nil`;
- `const x = nil` (nil всё равно не константа);
- любой другой контекст без expected type: `nil` как самостоятельное выражение,
  индекс, аргумент builtin без сигнатуры и т.п.
- явное указание типа там, где контекст есть, тоже разрешено: `f(nil[error])`.
  Это и есть вторая форма в одном и том же месте.

`checkVarDecl` (`internal/sema/check.go:523`) сначала разрешает значения, потом
объявляет имена; типа у объявления нет, поэтому `var p = nil` не получает
expected type. `checkPackageVars` (`internal/sema/resolve.go:438`) устроен так же.
Это единственная причина существования `nil[T]`.

### Codegen

`internal/gen/expr.go:47`:

- `NilLit` с типом → `(T)(nil)`;
- без типа → `nil` (после успешной sema почти недостижимо).

То есть `nil[T]` не создаёт отдельного значения: в Go уходит ровно `(T)(nil)`.
Для интерфейсов это даёт nil-интерфейс, для указателей — настоящий nil, не
`new(T)`. Форматтер (`internal/format/format.go:618`) печатает `nil` и, если тип
задан, `[T]`.

## 2. Варианты

### A. Только `nil[T]`

**Идея.** Убрать `applyNil`; любой `nil` без типа — ошибка. Парсер можно оставить
прежним: bare `nil` разбирается, но sema всегда отвергает его с подсказкой
`nil[T]`. Это даёт лучшую диагностику, чем запрет в парсере.

**Что меняется.** `internal/sema/expr.go`: удалить `applyNil` и девять вызовов,
упростить ветки `==`/`!=`; сообщение об ошибке. Codegen и форматтер уже умеют
`nil[T]`, их можно не трогать.

**Go interop.** Не ломается: `nil[error]`, `nil[tea.Cmd]`, `nil[*os.File]`,
`nil[context.Context]` разрешаются через `GoType`/`nilable`, в Go уходит
`(T)(nil)`. Но explicit type должен совпадать с ожидаемым. Если написать
`check(nil[*i32])` там, где параметр `any`, получится non-nil interface:
проверено на baseline, `check(nil[*i32])` печатает `non-nil`, а
`check(nil[any])` — `nil`. Sema это не ловит: `checkAssign` проверяет только
методы интерфейса. Это не ошибка компиляции, а смысловая ловушка.

**Ломается.**

- `internal/sema/new_syntax_test.go` — позитивный `TestBareNilExpected`
  (`return nil`, `e == nil`) станет ошибкой; тест нужно удалить или инвертировать.
- `tests/cases/multires/main.str:21,27,33,35` — четыре bare `nil`.
- `tests/smoke/bubbletea/main.str:8,23` — `return nil` / `return self, nil`;
  станет `nil[tea.Cmd]`.
- `internal/format/format_test.go:114` — sample `var x = nil` (только форматтер,
  но канонический пример надо обновить).
- `internal/parser/new_syntax_test.go` — `TestBareNil` продолжит проходить, пока
  парсер терпит bare; если запрещать в парсере, тест также ломается.
- `design/SPEC.md`, `design/SYNTAX.md`, `design/GAPS.md`, `docs/presentation/*` —
  правило и примеры.

Все `nil[T]` в тестах (`gen_test.go`, `par_gaps_test.go`, `nilmap`, `ctxcall`,
`parser_test.go`, `lexer_test.go`) остаются как есть.

**Оценка: S.** Полдня-день, в основном текст тестов и документации. Риск
минимальный, откат тривиальный.

### B. Только bare `nil`, без `nil[T]`

**Идея.** Парсер больше не принимает `[T]`; `applyNil`, `nilable` и codegen
остаются. `nil` везде получает тип из контекста.

**Что меняется.** `parseNilLit` перестаёт читать `[]`; форматтер всегда печатает
`nil`; AST-поле `Type` можно оставить внутренним (его заполняет `applyNil`),
тогда codegen не меняется. Ошибка для `var p = nil` остаётся, но текст надо
переписать без `nil[T]`.

**Go interop.** В вызовах, `return`, полях и сравнениях всё работает: sema
подставляет тип, codegen по-прежнему печатает `(T)(nil)`. Ломается объявление
nil-переменной:

```strizh
var p = nil;          // нельзя: нет expected type
var cmd = nil;        // нельзя: tea.Cmd — функция
var handler = nil;    // нельзя: функция
var err = nil;        // нельзя: интерфейс
```

Для map/slice есть пустые литералы (`map[K,V] {}`, `T[] {}`), для указателя,
функции и интерфейса — нет. Обойти можно только helper-функцией, которая
возвращает nil, или изменением смысла (`&T{}` — уже не nil). Это не «стиль», а
потеря выразительности, важная для Go interop: `var cmd tea.Cmd` — обычный
паттерн Bubble Tea.

**Ломается.**

- Все 13 вхождений `nil[T]` в коде/тестах:
  `internal/format/format_test.go:77`,
  `internal/gen/par_gaps_test.go:36,114,149`,
  `internal/gen/gen_test.go:439,460,568,592`,
  `internal/lexer/lexer_test.go:485`,
  `internal/parser/parser_test.go:559`,
  `tests/cases/nilmap/main.str:9,10`,
  `tests/cases/ctxcall/main.str:7`.
- Из них сравнения (`err == nil[error]`, `ctx != nil[context.Context]`) и
  `return 4, nil[error]` переписываются в bare и работают.
- `nilmap` (`var p = nil[*Node]`), `gen_test.go:568` (`var zero = nil[error]`) и
  `format_test.go:77` (`var p = nil[*User]`) в B выразить нельзя; тесты придётся
  менять по смыслу или удалять.
- `internal/parser/new_syntax_test.go` — `TestBareNil` станет единственной
  формой; нужен тест, что `nil[T]` отвергается.
- `design/SPEC.md`, `design/SYNTAX.md`, `design/GAPS.md`, `docs/presentation/*`.

**Оценка: S по коду, но продукт неполный.** Как промежуточный шаг годится, как
1.0 — нет: пользователь не может объявить nil-указатель или nil-функцию.

### C. Bare `nil` + typed `var`

**Идея.** Убрать `nil[T]`, но добавить тип в объявление:

```strizh
var p: *Node = nil;
var cmd: tea.Cmd = nil;
var handler: fn(i32) i32 = nil;
var e: error = nil;
```

`nil` остаётся один, тип пишется там, где объявляется переменная.

**Что меняется.**

- `ast.VarDecl` — поле `Type`.
- `parser.parseVarDecl` — после имён необязательное `: Type` перед `=`.
- `format` — печать `: T` в `varDecl` и `inlineVarDecl`.
- `sema.checkVarDecl` — разрешить объявленный тип, вызвать `applyNil(value, Type)`
  для каждого значения, объявить имя с этим типом. То же в `checkPackageVars`.
- `gen.declStmt`/`packageVar` — печатать тип: `var p *Node = ...`.
- `lsp.varDeclText`/hover — показывать тип (косметика).
- `parseNilLit` — убрать `[T]`; `format` — всегда `nil`.
- Codegen `NilLit` можно не менять: `applyNil` заполнит `Type`, и в Go уйдёт
  `(T)(nil)`; для typed var это даст `var p *Node = (*Node)(nil)` — валидный Go.

**Go interop.** Полный: `error`, `tea.Cmd`, `context.Context`, указатели и
функции объявляются и передаются как в Go. `var cmd: tea.Cmd = nil` превращается
в `var cmd tea.Cmd = (tea.Cmd)(nil)`, `cmd == nil` — в `cmd == (tea.Cmd)(nil)`.

**Edge cases, которые надо решить.**

- `var a, b: T = ...` — один тип на оба имени (как в Go); либо для 1.0
  разрешить typed var только с одним именем.
- `var a, b: T = f()` где `f` возвращает два значения — либо запретить,
  либо требовать, чтобы оба результата были `T`.
- `const c: T = ...` — не нужно для nil; проще запретить тип у `const`.
- Проверка не-nil значений `var p: *T = other` сейчас останется слабой:
  `checkAssign` проверяет только интерфейсы, остальное поймает `go build`.
  Если делать typed var «по-настоящему», это уже L, а не M.

**Ломается.**

- Те же 13 `nil[T]`-вхождений, что в B, но переписываются без потери смысла:
  - `nilmap`: `var p: *Node = nil`, сравнение `p == nil`;
  - `gen_test.go:568`: `var zero: error = nil`;
  - `format_test.go:77`: `var p: *User = nil`;
  - `ctxcall`, `par_gaps`, `gen_test.go:439,460,953`: просто `nil`.
- `internal/parser/parser_test.go` и `internal/lexer/lexer_test.go` — обновить
  синтаксис; `parser_test.go:722` проверяет `NilLit.Type` для `nil[*Node]`.
- `internal/parser/new_syntax_test.go:54` `TestBareNil` остаётся, но `nil[T]`
  должен стать ошибкой; нужен тест typed var.
- `internal/format/format_test.go` — round-trip typed var.
- `design/SPEC.md` (правило «var always has an initializer; the type comes from
  it»), `design/SYNTAX.md` (открытый вопрос про `var p = nil`), `GAPS.md`,
  `docs/presentation/*`.
- `internal/lsp` — hover/definition текста объявления.

**Оценка: M.** День-два с тестами и документацией; без полной проверки
assignability. Если typed var должен проверяться как параметры — L.

## 3. Сравнение

| | A: только `nil[T]` | B: только bare | C: bare + typed var |
|---|---|---|---|
| Один канонический nil | да | да | да |
| `var p = nil` | да, через `nil[T]` | нет | да, через `var p: *T = nil` |
| nil-функция в переменной | да | нет | да |
| Go interop `error`/`Cmd`/`Context` | да, явно | в контексте | в контексте |
| Обычный `err == nil` | нет, `nil[error]` | да | да |
| Новый синтаксис | нет | нет | typed `var` |
| Основная работа | sema + тесты | parser/format + тесты | AST/parser/format/sema/gen/LSP |
| Оценка | S | S (но неполный) | M |

## 4. Рекомендация

Для 1.0 — **C**: bare `nil` плюс `var p: *T = nil`.

Причины:

1. Устраняет корень проблемы, а не удобство: тип появляется в объявлении, где он
   и так нужен, а не в каждом литерале.
2. Оставляет один nil во всех выражениях: `return nil`, `err != nil`,
   `f(nil)`, `p == nil`. Это идиоматично для Go-кода и не заставляет писать
   `nil[tea.Cmd]` в каждом callback.
3. Не теряет ни одного interop-сценария: `error`, `tea.Cmd`, `context.Context`,
   указатели, функции.
4. `nil[T]` после этого не нужен: его единственной обязанностью было обойти
   отсутствие типа у `var`.

Если typed `var` не успевает в 1.0, выпускать **A**, а не B. A — самый дешёвый
и безопасный способ получить один nil, но он делает каждый nil явным и создаёт
ловушку `nil[*T]` в интерфейсной позиции. B в 1.0 выпускать нельзя: он отнимает
объявление nil-переменных, а это ломает Go interop сильнее, чем многословность A.

## 5. Что проверено

- Прочитаны parser/AST/sema/codegen/formatter и все вхождения `nil`/`nil[T]` в
  тестах и `tests/`.
- На baseline собрал `cmd/strizh` и прогнал probe: `check(nil[*i32])` в
  параметре `any` даёт `non-nil`, `check(nil[any])` — `nil`; это подтверждает
  ловушку explicit concrete type в A.
- Полный `bin/check` не завершён (тесты шли дольше лимита под параллельной
  нагрузкой), поэтому зелёный статус не заявляется.
