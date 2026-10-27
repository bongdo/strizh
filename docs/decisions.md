# Decisions

## 2026-10-27 — Compile to Go and let the Go type-checker validate

**Контекст:** Strizh targets Go's runtime and stdlib; implementing a complete
type-checker twice would duplicate `go/types`.
**Решение:** sema resolves names, imports, enums, `or`, `defer`, context and
`par`; codegen emits Go; `go build` validates the remaining types. Generated
code carries `//line` directives so errors point to `.str` files.
**Почему:** one source of truth for types, full Go interop from day one.
**Что отвергли:** a standalone Strizh type-checker and a non-Go backend.
**Цена:** diagnostics for Go type errors need a build, and generated code must
be clean enough not to add errors.

## 2026-10-27 — Enums are sealed interfaces with pointer variant structs

**Контекст:** sum types need exhaustive `match` and recursive variants.
**Решение:** `type Shape interface { _s_isShape() }`; each variant is a struct
`ShapeCircle` with a pointer receiver marker; construction wraps the pointer in
the enum interface; `match` is a type switch.
**Почему:** recursion works without boxing every field, `match` is a normal Go
type switch, and the marker seals the type.
**Что отвергли:** tagged structs and generated variant closures.
**Цена:** an enum cannot be zero-initialized; variants must be constructed.

## 2026-10-27 — `or` is statement-level

**Контекст:** `or` needs to run statements (`or return`, `or err { ... }`) and
bind an error.
**Решение:** `or` may be the whole initializer, right side, expression
statement, or return value; it cannot be nested inside another expression.
**Почему:** codegen is a straight-line prelude; no expression-level control
flow, no hidden temporaries in arbitrary places.
**Что отвергли:** `or` as a general postfix operator.
**Цена:** `f(g() or return)` must be split into two statements.

## 2026-10-27 — Block-scoped defer via an explicit stack

**Контекст:** `defer` must fire at the end of the nearest block, including
through `return`, `break`, `continue`, `or return` and panic.
**Решение:** every function with defers owns a `strizhrt.DeferStack`; `defer`
pushes a closure; block exits call `RunTo(mark)`; panic safety comes from one
function-level `defer RunTo(0)`.
**Почему:** conditional defers work, loops do not leak, and no closure frame
signals are needed.
**Что отвергли:** function-scoped defer and a closure per deferred block.
**Цена:** one slice of closures per function that uses defer.

## 2026-10-27 — Hidden context is computed by a call-graph fixpoint

**Контекст:** `par` and `within` need cancellation, but `context.Context` must
not appear in Strizh code.
**Решение:** a function receives a hidden `_s_ctx` parameter only when it or a
callee needs one; Go functions whose first parameter is `context.Context` get
it automatically; `within` rebinds it to a deadline context.
**Почему:** pure Strizh stays context-free without coloring every function.
**Что отвергли:** context on every function and explicit `ctx` parameters.
**Цена:** a context-needing function exposes the context at the Go FFI
boundary, and callbacks passed to Go cannot need one.

## 2026-10-27 — One compiler binary, no external tools at runtime

**Контекст:** build, run, test, format and LSP need the same parser and sema.
**Решение:** `cmd/strizh` contains all commands; `strizh build` writes Go and
execs `go build`; the LSP server is in the same binary and uses parser/sema
directly.
**Почему:** one install, one version, no plugin drift.
**Что отвергли:** a separate language server binary and a Node-based client
for the core.
**Цена:** the binary is larger and `go` must be installed to build projects.

## 2026-10-27 — pero-strizhed is a focused port, not a byte-for-byte rewrite

**Контекст:** pero is 27k lines of Go built on Bubble Tea; Strizh v1 has no
multi-return tuples, no `any` type switches, and no interface inheritance.
**Решение:** port the editor core (`internal/ed`), config, themes, a tcell UI,
search, undo and LSP diagnostics; use small Go shims for terminal events and
the background LSP transport.
**Почему:** the result is a working editor that exercises the language, not a
demo that cannot build.
**Что отвергли:** a mechanical 12k-line UI translation, Bubble Tea, and a
JavaScript-only editor.
**Цена:** no AI/ghost text, soft wrap, system clipboard or full LSP UI yet.

## 2026-10-27 — One canonical way per intent; Go is a foreign layer

**Контекст:** Go interop leaked method sets, context wrappers, Go types and
feature-parity work into the language surface. That creates the same
"many ways to do it" problem Go itself is criticized for.
**Решение:** app code has one canonical form per intent; Go is a foreign/FFI
layer. Complex Go libraries go through Strizh facades or adapters; raw Go is an
escape hatch. Simple imports are bare (`use fmt;`), struct literals are named,
propagation is `or return`, comments are `//` only.
**Почему:** external libraries stay available, while app code remains small.
**Что отвергли:** Go feature parity (full generics constraints, generic enums,
expression-level `or`, user variadics, doc comments, full IDE features) and
hidden context across arbitrary Go callbacks.
**Цена:** binding/facade code for complex libraries; some Go APIs are not
directly usable from app code.

## 2026-10-27 — `strizh check` checks project tests

**Контекст:** a type error in a `test` block was only caught by `go test`.
**Решение:** `strizh check` generates `zz_strizh_test.go` in memory and enables
`packages.Config.Tests`; LSP stays production-only for now.
**Почему:** `check` should validate the whole project before build/run.
**Что отвергли:** a separate `check --tests` switch; tests are part of validity.
**Цена:** cold `strizh check` is slower because it runs `go list -test`.
