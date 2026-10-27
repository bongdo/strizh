# Strizh — design

- [`GAPS.md`](GAPS.md) — the 0.2 gap list and the plan to close it.
- [`ONE_WAY.md`](ONE_WAY.md) — canonical forms and the “one way per intent” rule.
- [`FFI.md`](FFI.md) — the Go boundary: foreign subset, facades, context and `par`.
- [`NIL.md`](NIL.md) — nil-form audit and the typed-var option.
- [`SPEC.md`](SPEC.md) — the language as implemented (0.2 preview): files and packages,
  types, variables, errors, control flow, concurrency, tooling, limits.
- [`PAR.md`](PAR.md) — normative `par`, `par for` and `within` semantics.
- [`TYPECHECK.md`](TYPECHECK.md) — how `strizh check` and the LSP type-check
  generated Go.
- [`SYNTAX.md`](SYNTAX.md) — the original v0 design snapshot and open questions.
- [`../docs/decisions.md`](../docs/decisions.md) — why the compiler is shaped
  the way it is.
- [`IMPL.md`](IMPL.md) — compiler package contracts.

The language is a Go backend with Hare-like syntax, sum types and structured
parallelism. The compiler, formatter and LSP live in one binary (`strizh`).
