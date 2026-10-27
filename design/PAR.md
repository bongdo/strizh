# Strizh 0.2 — `par`, `par for`, `within`

Normative for 0.2. Where SPEC.md is silent or looser, this file wins. The
gaps G1–G9 in GAPS.md are the implementation work; §8 maps each rule to the
test that pins it. Errors quoted here are the substrings the tests assert.

## 1. Terms

**Worker** — the unit that runs concurrently.

- `par { s1; s2; }`: one worker per **top-level statement** of the `par` body.
  A top-level statement is an immediate element of the body block. Statements
  inside a nested `if`, `for`, `match`, `within`, plain block or function
  literal are *not* workers: they run sequentially inside the enclosing
  worker. A nested `par` starts its own workers and its own error set. Each
  top-level statement is its own scope: a local declared in one worker is not
  visible in another.
- `par for (...) { body }`: one worker per iteration; the whole body block is
  sequential inside one worker.
- **ord** — the worker's index: its spawn ordinal (0, 1, 2, …) for `par`,
  and for `par for` over a slice, array, `str` or C-style loop. Spawn order is
  iteration order for all of these, so "lowest ord" is "lowest index". For a
  `map` range every worker reports ord `-1` (§2).
- **Captured root** — a name declared outside the worker: local of the
  enclosing function, parameter, `self`, global. Names declared inside the
  worker are locals.

## 2. Error ordering and delivery

One `ParErrors` per `par`/`par for` statement. Only the winner is delivered;
the other errors are dropped.

- Indexed workers: winner = lowest ord.
- Map workers: no index, so every iteration reports ord `-1`; the winner is
  the first error that reaches `ParErrors` (completion order). This is
  documented as nondeterministic; do not rely on it.
- `ParErrors.First()` precedence: the lowest non-negative index wins; entries
  with `-1` are considered only when no non-negative entry exists, and then
  the earliest `Add` wins. An indexed error is never hidden by an unordered
  one. Same inputs and same arrival order → same winner.

Delivery:

- Every `par`/`par for` statement creates a cancelable context derived from
  the enclosing one, even when no worker can fail statically: a panic in a
  callee must still cancel the siblings. When no worker reports an error the
  cancel is called at block exit.
- `or return` in a worker: run the worker's deferred calls first; if a defer
  panics, recovery records that panic and cancels. Otherwise add the error at
  the worker's ord, cancel the shared context and leave the worker.
- `par`/`par for` with an error-returning enclosing function: after all
  workers finish, the statement is `err := <first>; if err != nil { return
  err }`; the enclosing block's deferred calls run by the normal rules.
- Enclosing function cannot return an error (`void` or no `or error`): the
  winning error is re-panicked at the `par` statement — `panic(err)`.
- `return`, `break`, `continue` still may not cross a `par` boundary.
  `or return` / `or panic` are the only ways out of a worker.

## 3. Panic

Every worker has a recovery boundary: `defer func() { if r := recover(); r !=
nil { ... } }()`, installed so that the worker's own `DeferStack` (if any)
runs before recovery.

- A panic escaping a worker becomes an error at the worker's ord, cancels the
  siblings and returns from the worker. `Wait` still completes.
- `panic(v)` written directly in a worker body is recovered at the worker
  boundary and becomes `*strizhrt.PanicError{Value: v}`. The runtime captures
  `debug.Stack()` in `NewPanicError`; the stack is the recovery stack and
  normally includes the user's panic site.
- A panic raised inside a callee (Strizh or Go), or by Go code itself (index
  out of range, nil deref), is converted by the same recovery boundary. Go has
  already unwound the callee frames, so the message is exact and the stack is
  the recovery stack.
- A panic from a deferred call replaces the worker's panic and becomes that
  worker's error; remaining deferred calls of that worker are skipped because
  the stack is drained before the calls run.
- Winner selection uses §2. A panic at ord 0 beats an error at ord 1 and the
  other way round.
- Delivery uses §2. In a `void` function the `par` statement re-panics with
  the `*PanicError` value: the process still exits like a Go panic (exit 2)
  and the recorded stack is printed.
- `or panic(msg)` and `or panic()` inside a worker are **not** Go panics:
  they add an error at the worker's ord, cancel and return. `AsError` returns
  the error as is when the argument is an error, otherwise wraps the value with
  `fmt.Errorf("%v", v)`. For `or panic()` the value is the underlying error.
  `or panic` makes the enclosing `par` fallible even when no worker uses
  `or return`.

Runtime API (normative; `runtime/source.go`, package `strizhrt`):

```go
type PanicError struct {
    Value any
    Stack []byte
}

func (e *PanicError) Error() string // panic: value, newline, stack
func NewPanicError(v any) *PanicError // captures debug.Stack() at the recovery point
func AsError(v any) error             // error as is, otherwise fmt.Errorf("%v", v)

type ParErrors struct{ /* mutex + slice */ }
func (p *ParErrors) Add(index int, err error)
func (p *ParErrors) First() error
```

## 4. Defer and `within` per worker

- Each worker owns a `DeferStack` (`var _s_d _s_rt.DeferStack` declared inside
  the goroutine). No worker ever touches the enclosing function's stack.
- A `defer` written as a top-level statement of a `par` body is itself one
  worker: the deferred call runs when that worker ends, not when the failing
  sibling ends. Put related statements in a block (`par { { defer f(); ... };
  }`) when the cleanup must belong to the same worker. This follows from §1:
  only a block is one worker.
- `defer` and `within` inside a worker push onto that stack, marked per block,
  and run LIFO at block exit. The worker stack drains at worker exit and
  before an error/panic is reported (`or return`, `or panic`, panic).
- `within D` derives from the worker's context (the `par` cancel context),
  pushes its cancel and restores nothing outside its block. Each worker has
  its own ctx variable, so one worker's deadline never changes another
  worker's context.
- Deferred calls obey the write rules of §5.
- A `within` deadline applies only to the worker; a sibling cancelling the
  shared context also cancels every worker's `within` context.

## 5. Write rules

Before codegen, for each worker body compute:

- `R(s)` — every captured root referenced by top-level statement/iteration
  `s`, including the base of a write target.
- `W(s)` — every captured root the statement may write.
- Alias: `var q = p;` or `q = p;` where `p` resolves to a captured root makes
  `q` an alias of that root inside the worker (transitive). Aliased writes are
  attributed to the root.

Plain `par`, reject:

1. A captured root in `W` of two or more top-level statements.
2. A captured root in `W(s1)` and in `R(s2)`, `s1 != s2`:
   `read by another par statement: <root>`.
3. Any write through a pointer-typed captured root — deref `*p`, field `p.f`,
   slot `p[i]`, a non-pure method `p.M()` when `p` is pointer-typed, or an
   alias of one of those. Always rejected, even with a single writer:
   `writing to <root> captured from outside par`. Without alias/escape
   analysis disjointness cannot be proven, so it is not attempted.
4. Any write to a slot of a captured `map`, whatever the index:
   `writing to map <root> inside par`.
5. `self`: always a write when the receiver root is `self`, including
   `self.M()`: `writing to self inside par`.

`par for`, reject:

1. The `self` rule and the map rule above.
2. Any write through a pointer-typed captured root (§5.3).
3. Any other write to a captured root, unless the target is a **disjoint
   slot**: `base[i]`, `base[i].f` or `base[i][j]` where every base of the
   index chain is a slice or array (never a map, never a pointer) and the
   outermost index expression contains the loop's iteration variable. The
   exemption is per write target.
4. Writes to iteration-body locals are free, including iteration variables
   declared by the loop (`var` init, range names). A name assigned by a
   C-style init (`par for (i = 0; ...)`) is not worker-local: a direct write to
   it in the body is rejected as a captured write (§6).

Allowed in both: reads of captured roots, reads of `self` fields, writes to
locals declared inside the worker. Writes to globals are always rejected in
both forms: a global is shared with the whole program, not only with the other
workers. A write to a captured *value* struct
(`c.n = v`) is a write to `c`, allowed under 1–2 above; a method call on a
captured value struct (`c.M()`, implicit `&c`) is also a write to `c`.

## 6. C-style `par for` with assignment init

`par for (i = 0; i < n; i += 1)`, `i` declared outside:

- `i` is an iteration variable of this loop: it is in `iter` for the disjoint
  slot exemption, but it is not worker-local. A direct write to `i` inside the
  body is rejected like any other captured write; reads see the per-iteration
  copy.
- Each worker captures a per-iteration copy before the spawn
  (`_s_iN := i`); body references resolve to that copy. The condition and post
  still use the outer `i`; after the loop `i` holds what the post left.
- Several simple-ident LHS (`i, j = 0, 0`) behave the same way.
- Any other init (call, field target, index target) is rejected in sema.
  Keep that error local to the `par for` init.

## 7. Known limits

- No interprocedural alias or escape analysis. Aliases made by a field read
  (`var q = p.next`), an index read or a call result are not tracked; only
  `var q = p` / `q = p`.
- A panic (direct or from a callee) carries the message and the recovery
  stack; the direct panic site is normally visible in that stack.
- Writes inside function literals are not analysed.
- A method call on a captured interface value is conservatively a write and
  is rejected. Mutations reachable through a pointer/interface passed to a
  free Strizh or Go function, or through Go callback state, are not tracked
  as writes.
- Two writes to different fields of one captured value struct, or to two
  literal slots of one captured slice, are rejected: one root, one writer.
- Struct copies that share a map or pointer field are caught through the field
  type when the write goes through `.m[...]` or `.p...`; slice-field copies
  are only checked for disjoint iteration slots and remain in the static
  limit.
- Map writes are rejected even for disjoint keys; use a slice indexed by the
  iteration variable. `sync`/`atomic` are an explicit trust boundary outside
  this static analysis: the checker does not model their happens-before. Use
  `go build -race` for that path.
- No dynamic checks in the compiler. `go build -race` (G11) stays the runtime
  oracle.
- Hidden context is not coloured across package boundaries or function values
  (G10), so a `within` deadline does not cross them.

## 8. Test map

| Gap | Pinned by |
| --- | --- |
| G1 | `internal/gen/par_gaps_test.go` `TestParPanicBecomesError`, `TestParPanicCancelsPlainPar`, `TestParPanicVoidMain` |
| G2 | `internal/sema/par_gaps_test.go` `TestParAllowsWithin`, `TestParAllowsDefer`; `internal/gen/par_gaps_test.go` `TestParWithinInside`, `TestParWithinCancels`; `tests/cases/par_within` |
| G3 | `internal/gen/par_gaps_test.go` `TestParDeferInside`; `tests/cases/par_defer` |
| G4 | `internal/sema/par_gaps_test.go` `TestParAllowsOrPanic`; `internal/gen/par_gaps_test.go` `TestParOrPanic` |
| G5 | `internal/sema/par_gaps_test.go` `TestParMapWriteRejected` |
| G6 | `internal/sema/par_gaps_test.go` `TestParPointerWriteRejected`, `TestParAliasWriteRejected`, `TestParMethodWriteRejected` |
| G7 | `internal/sema/par_gaps_test.go` `TestParReadWriteRejected`, `TestParWriteSetsAllowed` |
| G8 | `internal/sema/par_gaps_test.go` `TestParForCstyleIterVar`; `internal/gen/par_gaps_test.go` `TestParForCstyleCapture`; `tests/cases/par_capture` |
| G9 | `runtime/par_test.go`; `internal/gen/par_gaps_test.go` `TestParMapError` |

Messages the tests assert: `writing to map`, `captured from outside par`,
`read by another par statement`, `writing to self inside par`,
`cannot leave a par block`.
