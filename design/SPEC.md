# Strizh — language specification (0.2 preview)

This document freezes every question left open in [`SYNTAX.md`](SYNTAX.md).
Where this preview and the older v0 notes disagree, this document wins.
This is not a 1.0 promise: the gap list is in [`GAPS.md`](GAPS.md).

## 1. Source files and packages

- Source files end in `.str`, UTF-8.
- A package is one directory. Every `.str` file in it belongs to the same
  package. There is no package clause; the directory name is the package name.
- The project root is the directory holding `strizh.toml`:

```toml
name = "hello"
module = "example.com/hello"
```

- `module` is the Go module path used for generated code. If absent,
  `example.com/<name>` is used.
- The root package must be `package main` and contain `fn main`.
- Local imports use a path: `use "./internal/ed" as ed;`.
- Go imports use a Go import path: `use fmt;`,
  `use "charm.land/bubbletea/v2" as tea;`.
  A bare name without `/` is always a Go import. A quoted simple path
  (`use "fmt";`) is rejected; the simple form is canonical.
- Generated Go lives next to the sources as `zz_strizh.go` and
  `zz_strizh_test.go`. `.strizh/` holds generated support files.

## 2. Lexical rules

- `;` ends every statement and every block used as a statement.
- Comments: `// ...` only; block comments are rejected with a diagnostic.
- Identifiers: Unicode letters, digits, `_`; the first character is a letter or
  `_`. Names beginning with `_s_` are reserved for codegen.
- Visibility is by the first letter, exactly as in Go: `User` is public,
  `user` is private.
- Go keywords (`func`, `range`, `package`, `import`, `go`, `chan`, `select`,
  `switch`, `goto`, `fallthrough`) are rejected as Strizh identifiers.
- String escapes follow Go. Raw strings use backticks.
- Durations: `10ns`, `500us`, `2ms`, `30s`, `5m`, `1h`. A duration is a value
  of type `Duration` and is accepted where `Duration` is expected.

## 3. Types

### 3.1. Builtins

`void`, `bool`, `i8`, `i16`, `i32`, `i64`, `u8`, `u16`, `u32`, `u64`, `int`,
`uint`, `f32`, `f64`, `str`, `rune` (alias `i32`), `byte` (alias `u8`),
`usize` (alias `uint`), `isize` (alias `int`), `any`, `error`, `Duration`
(alias `i64`), `Type` (runtime type value, for reflection interop).

### 3.2. Constructors

| Form | Meaning |
|---|---|
| `T[]` | slice of `T` |
| `T[N]` | array of `N` elements; `N` is a constant expression |
| `T[U]` | generic instantiation; `U` is a type |
| `map[K, V]` | map |
| `*T` | pointer |
| `fn(A, B) R` | function value |
| `fn(A, B) (R, S)` | function value with several results |
| `fn(A, B) R or error` | fallible function value |
| `pkg.Type` | imported type |
| `(T)` | grouping |

`T[]` binds tighter than `*`: `*T[]` is a pointer to a slice. `(*T)[]` is a
slice of pointers.

### 3.3. Structs

```hare
type Buffer = struct {
    data: u8[],
    name: str,

    fn Write(p: u8[]) int or error = {
        data = append(data, p...);
        return len(p);
    };
};
```

- Fields and methods live in the type body. Commas separate fields.
- `self` is the implicit pointer receiver and is never declared.
- Field access is bare inside methods unless a parameter or local shadows it;
  then `self.field` is required.
- Methods always use a pointer receiver.
- A method may not have the same name as a field of its type.

### 3.4. Enums

```hare
type Shape = enum {
    Circle { radius: f64 },
    Rect { w: f64, h: f64 },
};
```

- A variant without fields is written `Empty,`.
- Variants are constructed as `Shape.Circle { radius = 2.0 }`.
- Variant names must be unique within a package so that bare `case` patterns
  are unambiguous; a qualified pattern `case pkg.Circle { ... }` is allowed.
- An enum is a sealed sum type. Its zero value is not constructible.
- `match` on an enum must be exhaustive.

### 3.5. Interfaces

Interfaces exist for Go interop and for explicitly declared contracts:

```hare
type Writer = interface {
    Write(p: u8[]) int or error;
};
```

Implementation is structural. No `implements`, no embedding.

### 3.6. Generics

```hare
type Stack[Item] = struct {
    data: Item[],

    fn Push(x: Item) void = {
        data = append(data, x);
    };
};

fn Map[In, Out](xs: In[], f: fn(In) Out) Out[] = { ... };
```

- Type parameters are written in `[...]` after the function or type name.
- Constraints do not exist: every parameter behaves like Go `any`.
  Operations that need a constraint (`<`, `+`, indexing) are rejected by Go.
  This is a deliberate limit, not a preview gap.
- Type arguments are inferred when possible; explicit arguments are written
  `Map[i32, str](xs, f)`.
- Generic methods cannot introduce their own type parameters.

### 3.7. Aliases

```hare
type UserId = u64;
type Handler = fn(Request) Response or error;
```

An alias does not create a new Go type.

## 4. Variables, constants, literals

```hare
var x = 1;
var xs = u8[] {1, 2, 3};
var a = u8[3] {1, 2, 3};
var u = User { Name = "Alice", age = 30 };
var m = map[str, User] {};
const Max = 100;
const Greeting = "hello";
```

- `var` always has an initializer; the type comes from it.
- `const` declares a compile-time constant. `const` may be used for array
  lengths and in `case` patterns.
- Composite literals always carry a type.
- Struct fields are named `field = value`; omitted fields get zero values.
- Map literals use `key = value`.
- `nil` takes its type from context: `return nil;`, `f(nil)`, `x = nil`.
- When there is no context, the type is explicit: `nil[*Node]`, `nil[error]`,
  `nil[u8[]]`. `nil` is valid only for pointers, slices, maps, functions and
  interfaces.
- `has(m, key)` reports key presence.
- `*p` reads a pointer and `*p = v` writes through it.

### 4.1. Builtins

| Builtin | Meaning |
|---|---|
| `len(x)` | length of a slice, array, map or string |
| `cap(x)` | capacity |
| `append(s, x, ...)` | grow a slice; `append(s, xs...)` spreads |
| `copy(dst, src)` | copy elements |
| `delete(m, k)` | remove a map key |
| `has(m, k)` | key presence |
| `min(a, b, ...)`, `max(...)` | numeric minimum/maximum |
| `runes(s)` | `str` to `rune[]` |
| `bytes(s)` | `str` to `u8[]` |
| `str(runes_or_bytes)` | conversion to `str` |
| `panic(msg)` | abort |
| `assert(cond)`, `assert_eq(got, want)` | checks for tests and code |

Conversions use the type as a function: `i32(x)`, `u64(x)`, `f64(x)`,
`str(runes)`, `bytes(text)`.

Multiple names are allowed for multi-result calls:

```hare
var value, ok = m["key"];
var n, err = f();
```

## 5. Functions

```hare
fn area(r: f64) f64 = {
    return 3.14159 * r * r;
};

fn pair() i32, bool = {
    return 1, true;
};

fn load() User, Order or error = { ... };

fn main() void or error = { ... };
```

- Result type is mandatory, except that `void` may be omitted in the current
  preview. `void` is written explicitly when present.
- Several results are a comma-separated list. This is not a tuple value:
  no tuple type, indexing or argument nesting. Allowed positions are `return`,
  `var a, b = f();` and `a, b = f();`.
- In a function type, several results are grouped: `fn(i32) (str, bool)`.
- The fallible form is `T or error`, `A, B or error` or `void or error`.
  `or` supports at most one non-error result.
- `main` may be `void` or `void or error`. An error prints to stderr and exits 1.
- Function literals are closures and the only local callable form; local named
  functions are deliberately absent.

```hare
fn(i: int, j: int) bool => i < j
fn(i: int, j: int) bool { return i < j; }
```

- Parameter types on function literals may be omitted when the expected
  function type is known: `sort.Slice(xs, fn(i, j) => xs[i].age < xs[j].age);`
- `return` is explicit. Every path in a non-void function must return.

## 6. Errors

`or` is a statement-level postfix operator on a call that returns `T or error`.

```hare
var f = os.Open(path) or return;
var f = os.Open(path) or err { ... };
os.Remove(path) or panic("cannot remove");
```

Forms:

- `call() or return;` — propagate to the caller.
- `call() or err { ... };` — handle here; `err` is the error value.
- `call() or panic("message");` — panic with the message.
- `call() or panic();` — panic with the error value.

`or` may appear only as the entire initializer of a `var`, the entire right
side of an assignment, an expression statement, or a `return` operand. It may
not be nested inside another expression.

`if (err != nil) { return err; }` (and the multi-result form) is rejected:
propagation has one canonical spelling, `or return`. `if err != nil` remains
valid for inspection, logging, retry and `errors.Is`; only the pure manual
propagation pattern is an error.

`defer` is block-scoped. Deferred calls run when the enclosing block exits,
including through `return`, `break`, `continue`, `or return` and panic.

## 7. Control flow

```hare
if (x > 0) {
    ...
} else if (x == 0) {
    ...
} else {
    ...
};

for (var i = 0; i < n; i += 1) {
    ...
};

for (var x in xs) { ... };
for (var i, x in xs) { ... };
for (var k, v in m) { ... };
for () { ... };

match (shape) {
case Circle { radius } => ...;
case Rect { w, h } => ...;
};

match (msg) {
case tea.KeyPressMsg as k => ...;
case tea.WindowSizeMsg => ...;
else => ...;
};
```

- `match` is exhaustive for enums. `else` is the default arm for other types.
- Type patterns are allowed on `any` and interfaces: `case T as name` binds the
  value, `case T` only tests the type. For open types `else` is required.
- `case _` is not used; write `else`.
- `match` is an expression when every case body is an expression; it is a
  statement when case bodies are blocks.
- `switch` does not exist. Use `if` or `match`.
- `break` and `continue` exist; labels do not.
- `for` conditions require parentheses.

## 8. Concurrency

```hare
par {
    users = loadUsers(id) or return;
    orders = loadOrders(id) or return;
};

par for (var i = 0; i < len(urls); i += 1) limit 64 {
    sizes[i] = fetchSize(urls[i]) or return;
};

within 30s {
    par for (var i = 0; i < len(urls); i += 1) {
        sizes[i] = fetchSize(urls[i]) or return;
    };
};
```

- `par` runs its top-level statements as workers and waits at `}`. Statements
  inside a nested block, `if`, loop, `match`, `within` or function literal run
  sequentially in the enclosing worker. A nested `par` starts its own workers
  and error set. Each top-level statement is its own scope; a local declared
  in one worker is not visible in another.
- `par for` runs iterations as workers with an optional `limit` (default
  `GOMAXPROCS`); the whole body is sequential inside one worker.
- Every `par`/`par for` creates a cancelable context derived from the enclosing
  one, even when no worker can fail statically. `within D` derives a deadline
  from the current worker's context.
- One `ParErrors` per statement. Indexed workers report their ordinal; the
  lowest ordinal wins. Map iterations report `-1`; then the first error that
  reaches the set wins, so the order is nondeterministic.
- `or return` in a worker adds the error at its ordinal, cancels the siblings,
  runs the worker's deferred calls and returns from the worker. After `Wait`
  the statement returns the winning error, or re-panics it in a `void`
  function. Inside a worker, `or panic` is an error with the same delivery, not
  a Go panic.
- A panic escaping a worker becomes a `PanicError` (value + stack), is added at
  the worker's ordinal, cancels the siblings, and `Wait` still completes.
- `defer`, `within` and `or panic` are allowed inside a worker. Each worker has
  its own `DeferStack`; its deferred calls run LIFO at worker exit and before an
  error or panic is reported. A top-level `defer` in a `par` body is itself a
  worker: wrap it in a block when the cleanup belongs to the sibling
  statements.
- `return`, `break` and `continue` may not cross a `par`/`par for` boundary.
  `or return` and `or panic` are the only ways out of a worker.
- Plain `par`: a captured root may be written by at most one top-level
  statement, and no other statement may read it. Writes through a captured
  pointer, to a captured map slot, to a global, or to `self` are rejected.
  Direct aliases
  (`var q = p`, `q = p`) are attributed to the root.
- `par for`: writes to captured roots are rejected except disjoint slots
  `base[i]`, `base[i].f` or `base[i][j]` where every base of the index chain is
  a slice or array and the outermost index uses the iteration variable. Map
  writes, pointer writes and `self` writes are always rejected.
- A C-style `par for` with an assignment init captures a per-iteration copy of
  each simple-ident LHS; the body sees the copy, while the condition and post
  use the outer variable. A direct write to that variable in the body is
  rejected as a captured write.
- `context` is invisible in Strizh. A function receives a hidden context only
  when it or its callees need one. Go functions whose first parameter is
  `context.Context` receive it automatically.
- On a Go interface boundary, codegen emits a public method with the exact Go
  signature and an internal context version. The user writes one method; the
  wrapper lives in `zz_strizh.go` (G10).
- Checks are static. `go build -race` stays the dynamic oracle (G11): writes
  through a pointer/interface passed to a free or Go function, writes inside
  function literals, and aliases other than `var q = p` are not tracked as
  writes. A method call on a captured interface value is conservatively a
  write.

## 9. Tooling

- `strizh build [-o out]` — translate and compile.
- `strizh run [args]` — build and run.
- `strizh check` — parse, check and type-check without building.
- `strizh test` — run `test` blocks with `go test`.
- `strizh fmt [-w] [paths]` — canonical formatter.
- `strizh lsp` — language server on stdio.
- `strizh new <name>` — scaffold a project.

Tests:

```hare
test "append grows the slice" {
    var xs = u8[] {};
    xs = append(xs, 1);
    assert_eq(len(xs), 1);
};
```

`assert(cond)` and `assert_eq(got, want)` are builtins. In tests they report a
failure with `file:line`; outside tests they panic.

## 10. Deliberately absent

These are not planned, not merely missing:

- local named functions; function literals are the only local callable form;
- generic constraints; every type parameter behaves like `any`;
- generic enums;
- guards and tuple/struct patterns in `match`; patterns stay literal, variant
  and type patterns;
- lambda parameter inference without an expected type;
- expression-level `or`; `or` is statement-level only;
- user-declared variadic functions; Go variadic functions can still be called
  with `...`;
- doc comments attached to declarations; comments are formatter input only.

`go`, `chan`, `select`, `async`, `await`, `try`, `catch`, `?`, `export`,
`let`, `:=`, attributes, traits, lifetimes, arenas, `unsafe` in application
code, method declarations outside a type, UFCS, `switch`, labeled control flow,
operator overloading, exceptions.

## 11. Known limits (0.2 preview)

- `or` is statement-level only. It is not allowed in package-level initializers.
- `break` inside a `match` case is rejected; use `return` or restructure the loop.
- Multi-result calls cannot be nested in other expressions; `or` handles at
  most one non-error result.
- Hidden context is not propagated across package boundaries or through
  function values; the public wrapper starts a fresh context there.
- `par` checks are static. There is no dynamic race detection in the compiler;
  `go build -race` is the oracle (G11). The analysis has no interprocedural
  alias or escape analysis: aliases from field/index reads and call results are
  not tracked; writes inside function literals are not analysed. A method call
  on a captured interface value is conservatively a write; a pointer or
  interface passed to a free/Go function is not tracked as a write.
  Map writes are rejected even for disjoint keys.
- A panic raised in a callee carries its message; `PanicError` keeps the value
  and a stack for diagnostics.
- A Strizh function that needs a hidden context is exposed through the
  public wrapper; a Go caller does not pass a context, and an outer Strizh
  deadline does not cross that boundary.
- Go functions returning `(T, bool)` must be handled with two-variable
  declarations, not `or`.
- `match` on `any` requires `else`; type patterns are limited to `case T` and
  `case T as name`.
- Enum variants share names with their generated Go types `<Enum><Variant>`.
- Interface conformance is checked for Strizh and Go interfaces, but signatures
  that cannot be named (generics, enum variants, arrays, function values and
  variadic methods) are left to `go build`; an explicit Go `context.Context`
  parameter is compared as an ordinary parameter. Multi-result assignments and
  returns are skipped.
