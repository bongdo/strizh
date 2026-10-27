# Strizh

Strizh is a small application language with Go's runtime and ecosystem:
Go-speed binaries, Hare-like syntax, sum types, and structured parallelism.

```hare
use fmt;

type User = struct {
    Name: str,
    age: u32,

    fn Greet() str = {
        return "hi, " + Name;
    };
};

fn main() void or error = {
    var u = User { Name = "Alice", age = 30 };
    fmt.Println(u.Greet()) or return;
};
```

## Install

```sh
./bin/build
export PATH="$PWD/bin:$PATH"
```

## Quick start

```sh
strizh new hello
cd hello
strizh run
strizh build
strizh test
```

## Status

The 0.2 preview compiler, formatter and language server work end to end. The
conformance suite in `tests/cases` covers structs, enums, generics, errors,
multiple results, type patterns, `defer`, `match`, local packages, Go interop,
hidden context wrappers and the `par` core (G1–G9: panic recovery, per-worker
`defer`/`within`, map/pointer/alias write rules, C-style capture). `bin/check`
runs it with the unit and LSP tests.

`strizh check` now type-checks the generated Go through `internal/typecheck`
and reports errors at `.str` positions; the LSP publishes the same diagnostics
on open and save. Sema also checks interface conformance (method sets) for
Strizh and Go interfaces.

Known limits and the full gap list are in [`design/GAPS.md`](design/GAPS.md):
statement-level `or`, no local named functions, no generic enums, no dynamic
race detection (G11), context loss across package/function-value boundaries,
test files are not type-checked by `strizh check`, and the remaining LSP work.
This is a 0.2 preview, not 1.0.

## Commands

| Command | What it does |
|---|---|
| `strizh build [-o out]` | translate `.str` to Go and compile a binary |
| `strizh run [-- args]` | build and run |
| `strizh check` | parse, check and type-check without building |
| `strizh test [go test flags]` | run `test` blocks with `go test` |
| `strizh fmt [-w] [paths]` | canonical formatter |
| `strizh lsp` | language server on stdio |
| `strizh new <dir>` | scaffold a project |
| `strizh init [dir]` | add `strizh.toml` to an existing directory |

## Project layout

```
hello/
  strizh.toml      name and Go module path
  main.str         package main
  lib/             package lib (optional)
    util.str
```

Local imports use a path:

```hare
use "./lib" as lib;
```

Go imports use a Go import path:

```hare
use fmt;
use "charm.land/bubbletea/v2" as tea;
```

The root package is compiled as `package main`. One directory is one package.
Generated Go lands next to the sources as `zz_strizh.go`; the support runtime
is copied to `.strizh/runtime`.

## Language

The full specification is in [`design/SPEC.md`](design/SPEC.md). The original
design notes and open questions are in [`design/SYNTAX.md`](design/SYNTAX.md).
Compiler internals are in [`design/IMPL.md`](design/IMPL.md).

Highlights:

- `var x = value;`, `const Name = value;`, no `:=`.
- `fn name(a: T) R = { ... };`, errors as `T or error`, propagation with `or`.
- multiple results: `fn f() A, B or error`, `var a, b = f();`.
- `type Name = struct { ... }`, fields and methods inside the body, implicit `self`.
- `type Shape = enum { Circle { radius: f64 }, Empty, };` with exhaustive `match`.
- `match` on `any` with `case T as x` and `else`.
- `T[]`, `T[N]`, `map[K, V]`, `*T`, `fn(A) R`, generics `Stack[T]`.
- `par { ... }`, `par for (...) limit N`, `within 30s { ... }`, no visible `context`.
- `defer` is block-scoped. Hidden context gets a non-ctx wrapper at Go interfaces.
- `par` workers recover panics, keep their own `defer` stack, and reject unsafe
  map/pointer/alias writes; `strizh check` type-checks generated Go.

## Presentation

`docs/presentation/index.html` — обзор языка, синтаксиса, Bubble Tea и внутренней
кухни. Рядом: `internals.html` (компилятор) и `cheatsheet.html` (шпаргалка).

## Editor support

- Neovim: see [`docs/lsp.md`](docs/lsp.md).
- VS Code: [`editors/vscode`](editors/vscode) has a TextMate grammar and a
  language client. Run `npm install && npm run compile` inside it.
- Any editor with an LSP client can use `strizh lsp`.

## Development

```sh
bin/check     # gofmt, go vet, unit tests, conformance cases
bin/build     # build bin/strizh
bin/run help  # run the compiler from source
```

Conformance programs live in [`tests/cases`](tests/cases). Each case has a
`strizh.toml`, a `main.str`, and an `expected.txt`.
