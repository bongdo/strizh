# Strizh — gap list (0.2 preview)

This is the honest list of what is missing or only partially implemented.
It is not a 1.0 checklist: it is the work needed before calling the language
finished.

**Status:** 0.2 preview. The compiler, formatter, LSP and tests work end to end.
0.2 adds multiple results, type patterns, bare `nil`, context wrappers for Go
interfaces, Go type diagnostics in `check`/LSP, interface conformance checks,
and the `par` core (G1–G9). The remaining concurrency gap is the dynamic race
oracle (G11).

Effort: **S** — hours, **M** — days, **L** — a week or more.

## What works today

- `.str` files, one directory = one package, `strizh.toml`, local and Go imports.
- structures with fields/methods and implicit `self`; enums with exhaustive
  `match`; interfaces; generics without constraints; aliases.
- `var`/`const`, slices/arrays/maps/pointers, `nil`/`nil[T]`,
  `runes`/`bytes`/`str` conversions, `len/cap/append/copy/delete/has/min/max`.
- multiple results: `fn f() A, B or error`, `return a, b;`, `var a, b = f();`.
- `match` on `any`/interfaces with `case T as name`, `case T` and `else`.
- `T or error` with `or return`, `or err { ... }`, `or panic(...)`;
  block-scoped `defer`; statement-level forms.
- `if`, all `for` forms, `match` statements/expressions.
- `par`, `par for ... limit`, `within`; panic recovery, per-worker
  `defer`/`within`, deterministic indexed errors, map first-completed errors,
  captured read/write rules. Aliases beyond direct assignment remain a static
  limit (G11).
- `strizh check` parses, checks and type-checks generated Go.
  `strizh build/run/test` still rely on `go build` for type errors.
- `strizh build/run/check/test/fmt/lsp/new/init`.
- LSP diagnostics, hover, definition, completion, symbols, signature help,
  formatting; VS Code grammar/client; Neovim setup.

## A. Concurrency (`par`, `par for`, `within`) — core closed

G1–G9 are fixed in 0.2; [`PAR.md`](PAR.md) is the normative description of the
implemented semantics. The remaining item is G11: the checks are static, and
the Go race detector stays the dynamic oracle.

### G1. Panic inside `par` — fixed in 0.2
Every worker runs inside a recovery boundary. A panic becomes a `*PanicError`
(value + stack) at the worker's ord, cancels the shared context and lets `Wait`
finish. An error-returning function returns the winning error; a `void`
function re-panics it at the `par` statement. `or panic` inside a worker adds
an error, cancels and returns instead of raising a Go panic.

### G2. `within` inside `par` — fixed in 0.2
Each worker owns its `DeferStack`. A `within` inside a worker derives from the
`par` cancel context, applies only to that worker, and sibling cancellation
still reaches it.

### G3. `defer` inside `par` — fixed in 0.2
Deferred calls go to the worker's own stack and run LIFO at worker exit, before
an error or panic is reported. A top-level `defer` in a `par` body is its own
worker; wrap it in a block when the cleanup belongs to the sibling statements.

### G4. `or panic` inside `par` — fixed in 0.2
`or panic(msg)` and `or panic()` add an error at the worker's ord, cancel the
siblings and return from the worker. They make the enclosing `par` fallible.

### G5. Map writes — fixed in 0.2
Any write to a slot of a captured map is rejected in `par` and `par for`, even
with the iteration variable as the index: `writing to map <root> inside par`.

### G6. Pointer and alias writes — fixed in 0.2
A write through a pointer-typed captured root (`*p`, `p.f`, `p[i]`, a non-pure
`p.M()`) and its direct aliases (`var q = p`, `q = p`) is rejected: `writing to
<root> captured from outside par`. A method proven pure is allowed. Writes to fields of a captured value struct are
attributed to the struct root. Aliases from field/index reads and call results
are not tracked; those remain in the static-analysis limit (G11).

### G7. Plain `par` read/write sets — fixed in 0.2
Each top-level statement is its own scope; a local declared in one worker is
not visible in another. For each top-level statement sema computes captured
read and write sets. A root may have one writer, and no other statement may
read it:
`read by another par statement: <root>`. Writes to `self` and to globals are
always rejected; a global is shared with the whole program.

### G8. C-style `par for` capture — fixed in 0.2
`par for (i = 0; ...)` with an outer `i` captures a per-iteration copy
(`_s_iN := i`); the body sees the copy while the condition and post use the
outer `i`. A direct write to `i` in the body is rejected as a captured write.
Iteration variables declared by `var` init or range are worker-local: writes to
them are allowed. Non-identifier init targets are rejected by sema.

### G9. Map error order — fixed in 0.2
Indexed workers report their spawn ordinal, so the lowest index wins. Map
iterations report ord `-1`; `ParErrors.First()` prefers any non-negative index
and otherwise takes the first completed error. Map order is documented as
nondeterministic. Map aliases and Go map types are recognized by `rangeTypes`,
so their range yields keys and their `par for` uses `-1`.

### G10. Context coloring at the Go boundary
**Статус:** fixed in 0.2 for direct calls, methods and Go callbacks.
**Сейчас:** a function or method that needs a hidden context gets a public
wrapper with the exact Go signature and an internal `_s_..._ctx` body. The
wrapper calls the internal function with `context.Background()`; Strizh calls
inside a `within`/`par` pass the real context. Internal functions called across
package boundaries go through the public wrapper.
**Остаток:** the limit is the boundary itself. Interface dispatch through a
Strizh interface, package-level initializers and function values are not
context-colored: a call through them uses the public wrapper with
`context.Background()`, so a `within` deadline does not cross a package
boundary or a function value. Timers and IO are executed by `tea.Program`, not
by Strizh.

### G11. `par` checks are static only
**Сейчас:** `strizh build -race` can be passed to Go, but nothing forces it.
**Почему:** the Strizh compiler has no runtime instrumentation.
**Фикс:** add `strizh run -race` / `strizh test -race` documentation and a
`strizh check --race` path; keep Go's race detector as the dynamic oracle.
**Оценка:** S.

## B. Language surface

### G12. Local named functions are out of scope for 0.2
**Сейчас:** `fn helper(...)` inside a function body is a parse error. Only
top-level functions and function literals exist.
**Решение:** not planned for 0.2. Function literals cover callbacks and
closures; a named local function is a separate feature with recursion and
capture rules. SYNTAX.md no longer promises the old example.

### G13. Multiple results are implemented, tuples are not
**Статус:** fixed in 0.2.
**Синтаксис:** `fn f() A, B or error`, `return a, b;`, `var a, b = f();`,
`a, b = f();`. Function types use `fn(i32) (str, bool)`.
**Границы:** no tuple type, no tuple values, no indexing or nesting in
arguments. `or` supports at most one non-error result. `return f();` passes a
fallible multi-result call through. This unlocks `tea.Model` and Go APIs.

### G14. `match` on `any` and interfaces
**Статус:** fixed in 0.2.
**Синтаксис:** `case T as name => ...` binds, `case T => ...` tests only,
`else => ...` is the default. Enums keep variant patterns. Open types require
`else`.
**Границы:** no guards, no tuple/struct patterns, no multi-type case lists.

### G15. Generic constraints are missing
**Сейчас:** every type parameter behaves like `any`; `<`, `+`, indexing and
similar operations fail in Go.
**Почему:** no syntax and no constraint model.
**Фикс:** `fn F[T: comparable]`, `T: numeric`, map constraints to Go type sets;
sema validates operations; codegen emits Go constraints.
**Оценка:** L.

### G16. Generic enums are rejected
**Сейчас:** sema error `generic enums are not supported`.
**Почему:** variant construction and type switches would need type arguments.
**Фикс:** parameterize the enum interface and variant structs (`Shape[T]`,
`ShapeCircle[T]`), carry type args on construction/patterns, instantiate at
use sites. Alternatively keep the restriction and remove it from the design
promise.
**Оценка:** M.

### G17. Match patterns are limited to literals and simple names
**Сейчас:** `case -1`, `case N+1`, tuple/struct patterns and guards are not
supported.
**Почему:** parser patterns are literal or identifier/variant only.
**Фикс:** parse const expressions and optional `if` guards; resolve constants;
duplicate detection; codegen case expressions.
**Оценка:** M.

### G18. Type arguments on calls are not resolved by sema
**Сейчас:** `Map[i32, str](...)` is parsed and codegen reparses raw types; sema
does not resolve `Call.TypeArgs`, so hover/completion/inference are blind.
**Почему:** call resolution ignores `TypeArgs`.
**Фикс:** resolve each `TypeArg` with `resolveType`; attach the generic
declaration and instantiated types to `CallInfo`.
**Оценка:** S.

### G19. Lambda parameter inference needs an expected type
**Сейчас:** `fn(i, j) => ...` works as a Go/Strizh callback argument, but
`var f = fn(x) => x` fails.
**Почему:** no bidirectional inference for assignments without a type.
**Фикс:** infer from the body where possible, or require explicit parameter
types and give a targeted error. Document the rule.
**Оценка:** S/M.

### G20. `nil` typing
**Статус:** fixed in 0.2.
**Сейчас:** bare `nil` is allowed when the expected type is known; otherwise
`nil[T]` is required. Only pointers, slices, maps, functions and interfaces are
accepted. Generated nil is `(T)(nil)`, not `*new(T)`, so the value is a real
nil.

### G21. `break` inside a `match` case is rejected
**Сейчас:** sema error, because Go's `break` would leave the switch, not the
loop.
**Почему:** codegen does not label loops.
**Фикс:** emit labels for loops and rewrite `break`/`continue` inside a match
to `break _s_loop`/`continue _s_loop`; or keep the restriction and document it.
**Оценка:** S.

### G22. `or` is statement-level only
**Сейчас:** `f(g() or return)` is rejected; `or` must be the whole initializer,
RHS, expression statement or return value.
**Почему:** no expression-level control-flow lowering.
**Фикс:** add an AST pass that hoists fallible calls into temporaries before
statements; then `or` can appear anywhere. Alternatively keep the documented
restriction; the current SPEC already says statement-level.
**Оценка:** M.

### G23. String indexing returns a byte
**Сейчас:** `s[i]` is Go's byte; users must call `runes(s)` for rune work.
**Почему:** `str` maps directly to Go `string`.
**Фикс:** document the byte rule, or add `at(s, i) rune` / `runes(s)` as the
preferred form; changing `s[i]` to rune would surprise Go users.
**Оценка:** S.

### G24. User-declared variadic functions are missing
**Сейчас:** Go variadics can be called with `...`, but `fn f(xs: T...)` is not
in the grammar.
**Почему:** parameters are fixed-arity only.
**Фикс:** add `param...` to the AST, sema and codegen (`xs ...T`); calls with
spread. Go interop already has the call-side support.
**Оценка:** S/M.

### G25. Doc comments are not attached to declarations
**Сейчас:** the lexer/parser preserve comments for the formatter, but sema/LSP
do not know which comment documents which symbol.
**Почему:** comments are a flat file list.
**Фикс:** attach leading comments to `Decl` nodes; include them in hover and a
future `strizh doc`.
**Оценка:** S.

## C. Sema and type checking

### G26. Type errors in `check` and LSP — fixed (prototype) in 0.2
**Сейчас:** `strizh check` runs `internal/typecheck.Check` after a clean
parse/sema pass. It generates Go in memory, loads it through `go/packages` with
an overlay, and maps Go diagnostics back through `//line`; `strizh check` exits
1 on `var n = 1 + "x";`. The LSP runs the same check on `initialize`,
`didOpen` and `didSave` and publishes diagnostics with source `strizh`.
**Границы:** `build/run/test` still rely on `go build` for type errors; test
files (`zz_strizh_test.go`) are not checked; diagnostics have no columns (G34);
the LSP call is synchronous (G43). [`TYPECHECK.md`](TYPECHECK.md) is the
detailed contract.
**Остаток:** async/debounce (G43), columns (G34), test packages, and
per-package cache.

### G27. Interface conformance — fixed in 0.2
**Сейчас:** sema computes method sets for local structs, local interfaces and
Go types. Assignments, arguments, returns and composite elements are checked at
the source position: missing method, wrong parameter/result, missing error,
unexported cross-package method, value receiver where a pointer is required
(`does not implement Writer: ...`).
**Остаток:** signatures that cannot be named are skipped rather than guessed:
generic types and type arguments, enum variants, array types, function values
and variadic methods. An explicit Go `context.Context` parameter is compared as
a normal parameter; a hidden-ctx implementation does not match it. Multi-result
assignments/returns are currently skipped because the call info has several
results.

### G28. Enum GoName collisions are checked in map order
**Сейчас:** `ShapeCircle` colliding with a later type may be missed depending
on map iteration order.
**Почему:** collision check happens during collection, before all types exist.
**Фикс:** final package-wide symbol table pass after collection; also detect
variant names that make bare patterns ambiguous.
**Оценка:** S.

### G29. Top-level forward references have no inferred type
**Сейчас:** `var a = b; var b = 1;` gives `a` no sema type; hover/completion
are weak, even though Go compiles it.
**Почему:** one pass over package vars.
**Фикс:** build a dependency graph, iterate type inference to a fixpoint, detect
cycles.
**Оценка:** M.

### G30. `void` is not fully restricted
**Сейчас:** sema rejects void fields/params, but some expression positions can
still reach Go with no value.
**Почему:** the type is just a builtin named type.
**Фикс:** a dedicated `VoidType` that is allowed only as a function result;
reject in vars, fields, params, args, returns with values.
**Оценка:** S.

### G31. Unused locals/dead code — partly fixed by G26
**Сейчас:** `strizh check` type-checks generated Go, so unused locals and
similar Go diagnostics are reported there; the LSP gets them on open/save.
**Остаток:** no fast unused-local pass for LSP, and dead-code/vet diagnostics
are not a separate Strizh pass.
**Оценка:** S/M.

## D. Codegen and runtime

### G32. Generated Go can contain unreachable `panic("unreachable")`
**Сейчас:** every function with a result gets a trailing panic; `go vet` warns
`unreachable code`.
**Почему:** no definite-return analysis.
**Фикс:** compute whether a block always returns; emit the panic only when a
path can fall through.
**Оценка:** S.

### G33. Duplicate std imports when the user imports `fmt`/`os`
**Сейчас:** generated code can contain both `_s_fmt "fmt"` and `fmt "fmt"`;
Go accepts it, but it is noise.
**Почему:** fixed generated aliases and user aliases coexist.
**Фикс:** build a path→alias table; use the user alias for generated code when
it already imports the same path.
**Оценка:** S.

### G34. `//line` has no columns and uses absolute paths
**Сейчас:** errors point at a line, not a column; paths with spaces are fragile;
generated files embed machine paths.
**Почему:** `writeLineDirective` emits `path:line`.
**Фикс:** emit `//line path:line:col`; define a relative-path convention and
map it in the driver/LSP; escape or reject spaces.
**Оценка:** S.

### G35. `GoType.Text` can reference packages that are not imported
**Сейчас:** inferred Go callback types (e.g. `fs.DirEntry`) may print a package
name without an import, breaking generated code.
**Почему:** `types.TypeString` uses a qualifier that falls back to the package
name.
**Фикс:** collect required imports while printing Go types; add them to the
file import set; fail with a Strizh error if a type cannot be named.
**Оценка:** M.

### G36. `or` in multiple RHS / a `for` post is only rejected
**Сейчас:** sema errors instead of lowering; no fallback.
**Почему:** statement-level codegen.
**Фикс:** after G22, the hoisting pass handles these positions; until then keep
the clear error.
**Оценка:** covered by G22.

### G37. Generation always rewrites whole files
**Сейчас:** each build writes `zz_strizh.go` for every package; there is no
manifest of generated files and no cleanup for deleted packages.
**Почему:** simplest correct approach.
**Фикс:** write only when bytes change; keep `.strizh/generated.json`; remove
stale outputs. Low priority.
**Оценка:** S.

### G38. Runtime is copied on every build
**Сейчас:** `.strizh/runtime/runtime.go` is overwritten each time.
**Почему:** guarantees the runtime matches the compiler.
**Фикс:** compare content/hash; keep the current behavior otherwise.
**Оценка:** S.

## E. Tooling and LSP

### G39. LSP type diagnostics — partly fixed by G26
**Сейчас:** `didOpen`, `didSave` and `initialize` run the Go type-check and
publish mapped diagnostics with source `strizh`; `didChange` clears them until
the next save. Parser/sema diagnostics work as before.
**Остаток:** async/debounce and cancellation (G43), columns (G34), test files,
and publishing errors for generated code that is not covered by `//line`.

### G40. Completion is context-limited
**Сейчас:** package members after `pkg.`, local names, keywords and builtins.
No field completion after `u.`, no enum variants after `Shape.`, no
context-aware keywords.
**Почему:** no expression-type lookup at the cursor.
**Фикс:** find the node at the cursor; use `sema.ExprTypes`/`Ref` to resolve
struct fields, methods, enum variants and module members; add snippets.
**Оценка:** M.

### G41. Hover/definition do not work for selectors and Go symbols
**Сейчас:** hover works for simple idents/functions/types; definition returns
null for Go objects and some range variables.
**Почему:** no `go/types` file set/source positions; selector hover is not
implemented.
**Фикс:** load Go packages with `NeedSyntax`/`Fset`; store object positions;
resolve selector types; add hover for `SelectorExpr`/`NamedType`.
**Оценка:** M.

### G42. No rename, references, code actions, semantic tokens or folding
**Сейчас:** only the basic LSP set.
**Почему:** not implemented.
**Фикс:** symbol index + reference walk; semantic token encoder; folding from
blocks/comments; quickfixes for unused imports and `or`.
**Оценка:** L.

### G43. LSP analysis is synchronous and has no debounce/cancellation
**Сейчас:** a `didChange` blocks the message loop while sema walks the project;
a large project stutters.
**Почему:** no worker/versioned analysis.
**Фикс:** analysis worker with a latest-version queue, cancellation, and
debounced Go typecheck; publish only current versions.
**Оценка:** M.

### G44. LSP shim drops `didChange` when its queue is full
**Сейчас:** 64 outbound messages; under fast typing an update can be dropped
and diagnostics lag until the next edit.
**Почему:** non-blocking design with a bounded queue.
**Фикс:** coalesce per URI (keep only the latest text) instead of dropping;
make the queue larger or unbounded for control messages.
**Оценка:** S.

### G45. LSP does not clear diagnostics for closed project files
**Сейчас:** closed files are analyzed from disk; only deleted/standalone docs
are cleared.
**Почему:** reload publishes for all project files.
**Фикс:** publish empty diagnostics on `didClose`; keep a URI→version map and
drop stale publishes.
**Оценка:** S.

### G46. Formatter comment fidelity and wrapping
**Сейчас:** comments are preserved by line but can move to the next line;
there is no line-width wrapping.
**Почему:** the formatter prints comments before the next node.
**Фикс:** attach trailing comments to the previous node; add a golden corpus;
optional `--wrap` later.
**Оценка:** M.

### G47. VS Code extension is not packaged or integration-tested
**Сейчас:** TypeScript compiles; `npm run compile` works; no `.vsix`, no test in
a real VS Code instance.
**Почему:** not part of the core check.
**Фикс:** `@vscode/test-electron`, a smoke workspace, `vsce package`, CI.
**Оценка:** M.

### G48. CLI lacks several expected commands
**Сейчас:** no `strizh doc`, `clean`, `watch`, `vet`, `fmt -l/-d`, stdin mode,
or install script.
**Почему:** the minimal command set was enough for the language.
**Фикс:** add `clean` (remove generated), `fmt -l/-d`, `doc` (from comments),
`vet` (Go vet with mapped paths); watch is optional.
**Оценка:** M.

## Bubble Tea

Bubble Tea is Go and is callable from Strizh. 0.2 removes the two language
blockers:

- `Update` returns `(tea.Model, tea.Cmd)` — multiple results (G13).
- messages are `any` and are handled with `case T as name` (G14).

No Go adapter is needed. `*Model` implements `tea.Model` structurally; if
`Update`/`View` use `par`/`within`, G10 generates a public method with the exact
Go signature and an internal context version. `Cmd` is executed by
`tea.Program`. The compile-only smoke lives in
`tests/smoke/bubbletea` and pins `charm.land/bubbletea/v2 v2.1.0`; soft wrap,
clipboard, themes and completion are separate UI work.

## Suggested order

0.2 was the vertical slice: multiple results, type patterns, bare `nil`, G10,
G26/G27 and the `par` core. Everything else is deliberately later.

1. G11: dynamic race oracle (`-race` docs and path). G1–G9 are closed.
2. G15–G25: the remaining language surface — constraints, generic enums,
   richer patterns, type arguments, inference, `break` in `match`,
   expression-level `or`, string indexing, variadics, doc comments. G13, G14
   and G20 are already fixed.
3. G32–G38: codegen and runtime cleanup.
4. G39–G48: LSP quality and tooling; G39 is partly closed by G26.

## Commands

CLI:

```sh
strizh build [-o out] [go build flags]
strizh run [-- program args]
strizh check
strizh test [go test flags]
strizh fmt [-w] [paths]
strizh lsp [--root dir]
strizh new <dir>
strizh init [dir]
strizh version
strizh help
```
