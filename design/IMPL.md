# Strizh — compiler implementation notes

Internal contract between packages. Not user documentation.

## Pipeline

```
driver → parser → sema → gen (Go source) → go build / go test
                     ↘ typecheck (check, LSP)
                     ↘ format
                     ↘ lsp
```

## Package APIs

### `internal/lexer`

```go
type Comment struct { Pos, End token.Pos; Text string; Block bool }
type Error struct { Pos token.Pos; Msg string }
func Lex(src []byte) ([]token.Token, []Comment, []Error)
```

### `internal/parser`

```go
type Error struct { Pos token.Pos; Msg string }
func Parse(path string, src []byte) (*ast.File, []Error)
```

`ast.File.Comments` is filled. `ast.File.Path` is the absolute path.
Every node has a valid `Span`; `End` is exclusive.
The parser does not resolve names and does not reject Go reserved words.

### `internal/sema`

```go
type Error struct { Path string; Pos token.Pos; Msg string; Warning bool }

type GoPkg struct {
    Path  string
    Name  string
    Types *types.Package
}

type Import struct {
    Alias string
    Path  string
    Local *Package
    Go    *GoPkg
}

type Package struct {
    Rel     string
    Dir     string
    Name    string
    IsMain  bool
    Files   []*ast.File
    Imports map[string]*Import
    Types   map[string]*ast.TypeDecl
    Funcs   map[string]*ast.FuncDecl
    Vars    map[string]*ast.VarDecl
    Decls   []ast.Decl
    GoImports []string
}

type Program struct {
    Root     string
    Module   string
    Packages map[string]*Package
    Main     *Package
    Errors   []Error
}

type Options struct {
    Root   string
    Module string
}

func Check(parsed map[string][]*ast.File, opt Options) *Program
```

`parsed` keys are slash-separated directories relative to the root; root is `"."`.
`sema` loads Go packages lazily through `go/packages` with `Dir = Root`.
The driver guarantees `go.mod` exists and Go imports are resolvable before
`Check` is called; sema returns an error if they are not.

Sema fills:

- `Ident.Ref`, `SelectorExpr.Ref`
- `ast.Use.GoPath`, `PackageName`, `LocalDir`, `LocalPackage`
- `ast.NamedType.Decl/Variant/GoName/PkgAlias`
- `ast.CompositeLit.Variant`, `CompositeEntry.Field`
- `ast.CallExpr.Info` (including `ResultTypes`)
- `ast.FuncLit.ParamTypes`, `NeedsCtx`, `GoCallback`
- `ast.FuncDecl.Results`, `NeedsCtx`, `GoName`
- `ast.IFaceMethod.Results`, `NeedsCtx`
- `ast.FuncType.Results`, `NeedsCtx`
- `ast.MatchExpr.ResultType`, `IsExpr`, `SubjectType`, `TypeSwitch`
- `ast.Case.Pat.Variant`, `FieldDecls`, `Default`
- `ast.Pattern.TypePat`, `Bind`
- `ast.TestDecl.GoID`
- `ast.TypeDecl.GoName`, `ast.Variant.GoName`
- `ast.ArrayType` normalization from `GenericType` when a single argument is a
  constant expression

Sema also checks interface conformance (`internal/sema/iface.go`): method sets
for local structs, local interfaces and Go types, compared at assignments,
arguments, returns and composite elements. Hidden context is not added to the
compared Strizh signature; an explicit Go `context.Context` parameter is
compared as an ordinary parameter. Signatures that cannot be named (generics,
enum variants, arrays, function values, variadic methods) are skipped and left
to `go build`; multi-result assignments/returns are also skipped.

Sema checks Strizh-specific rules only. Go checks the rest of the types.

### `internal/gen`

```go
type File struct { Path string; Src []byte }
type Options struct { Root, Module string; IncludeTests bool }
func Generate(p *sema.Program, files map[string][]*ast.File, opt Options) ([]File, []error)
```

Outputs one `zz_strizh.go` per local package and, if `IncludeTests`, one
`zz_strizh_test.go` per package with test declarations.
Generated files are formatted with `go/format`.
Every declaration, statement and expression gets a `//line <abs path>:<line>`
directive where practical (at least decls, statements, and call expressions).
The absolute path and missing column are known limits (G34).

### `internal/typecheck`

```go
type Error struct { Path string; Pos token.Pos; Msg string }
func Check(p *sema.Program, files map[string][]*ast.File, opt Options) []Error
```

Generates Go in memory with `internal/gen`, overlays every `zz_strizh.go` and
the embedded runtime into `go/packages`, and maps `//line` positions back to
`.str` files. The load mode is `LoadImports | NeedExportFile`, not `LoadTypes`.
Results are cached in memory by a SHA-256 of generated files, `go.mod`,
`go.sum` and the runtime source. `strizh check` and LSP call it; `build`,
`run` and `test` still let `go build`/`go test` find type errors. Details:
[`TYPECHECK.md`](TYPECHECK.md).

### `internal/format`

```go
func File(f *ast.File) []byte
```

Canonical source. Tabs are not used; indentation is four spaces.
Comments are preserved by line.

### `internal/lsp`

```go
func Run(stdin io.Reader, stdout io.Writer, stderr io.Writer, root string) error
```

### `internal/driver`

High-level operations used by the CLI. The driver owns `strizh.toml`,
`go.mod`, the Go import stub, parsing, checking, generation and Go commands.

## Codegen rules

### Type mapping

| Strizh | Go |
|---|---|
| `str` | `string` |
| `u8`…`u64` | `uint8`…`uint64` |
| `i8`…`i64` | `int8`…`int64` |
| `int`/`uint` | `int`/`uint` |
| `usize`/`isize` | `uint`/`int` |
| `f32`/`f64` | `float32`/`float64` |
| `rune`/`byte` | `rune`/`byte` |
| `Duration` | `time.Duration` |
| `Type` | `reflect.Type` |
| `T[]` | `[]T` |
| `T[N]` | `[N]T` |
| `map[K,V]` | `map[K]V` |
| `*T` | `*T` |
| `any` | `any` |
| `error` | `error` |
| `fn(A) R` | `func(A) R` |
| `void` | no result |
| `T or error` | `(T, error)` |

### Packages and imports

- Generated Go package name = sanitized directory base (`-` → `_`), root is
  `main`.
- Local import `use "./ed" as ed;` → `<module>/ed` with alias `ed`.
- Go import keeps its path, alias from the `use` declaration or the real Go
  package name.
- Imports are merged per package; conflicting aliases are a sema error.
- Generated support package: `<module>/.strizh/runtime`, imported only when
  needed (defer stack, asserts, `has`, par helpers).

### Main wrapper

`fn main() void or error` generates:

```go
func main() {
    if err := _s_main(context.Background()); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

The real body is named `_s_main` when a wrapper is needed. A `void` main gets
a plain wrapper. If `main` does not need a context, no context is passed.

### Hidden context

- A function with `FuncDecl.NeedsCtx` is emitted as two Go functions:
  - public `Name(...)` with the exact source signature;
  - internal `_s_Name_ctx(_s_ctx, ...)` (or `_s_Type_Name_ctx`) with the body.
- The public wrapper calls the internal function with
  `context.Background()`. Direct Strizh calls inside `par`/`within` call the
  internal function with the current `_s_ctx`.
- Methods follow the same rule: the public method keeps the Go interface
  signature, the internal free function holds the body.
- A call to a foreign package or through a function value uses the public
  wrapper; a deadline does not cross that boundary.
- Calls to Go functions whose first parameter is `context.Context` insert
  `_s_ctx` and `CallInfo.NeedsCtx` is true.
- `ParStmt`, `par for` with fallible bodies, and `WithinStmt` need a context.
- A function literal passed to Go that needs a context gets
  `context.Background()` and is marked `GoCallback`; it can use `par` without
  inventing a parameter.
- `main` starts with `context.Background()`.

### `defer`

Block-scoped defer is implemented with a per-function stack:

```go
var _s_d runtime.DeferStack
defer _s_d.RunTo(0)
```

- At block entry: `_s_mark := _s_d.Mark()`.
- `defer f(args);` → evaluate receiver and args into temps, then
  `_s_d.Push(func() { f(a1, a2) })`.
- At normal block exit: `_s_d.RunTo(_s_mark)`.
- On `return`: `_s_d.RunTo(0)` before the return.
- On `break`/`continue`: run to the mark of the loop body being left.
- On `or return` / `or panic`: same as return/panic.
- `within` pushes `cancel` onto the same stack.
- Each `par`/`par for` worker declares its own `_s_d` inside the goroutine and
  runs `RunTo(0)` at worker exit. It never touches the enclosing function's
  stack.

### Errors (`or`)

- `var x = f() or return;` → `_s_v, _s_err := f(); if _s_err != nil { cleanup; return zero, _s_err }; x := _s_v`
- For a `void or error` function: `return _s_err`.
- For `T or error`: zero value `*new(T)`; a nil result uses `(T)(nil)`.
- Multiple non-error results: `return a, b, nil`; `return a, b` appends nil;
  `return f()` is a raw pass-through when the arities match.
- `or err { ... }` → `_s_v, _s_err := f(); if _s_err != nil { err := _s_err; ... }`.
- `or panic("msg")` → `if _s_err != nil { panic("msg") }`.
- Direct call with no assignment and only an error result:
  `if _s_err := f(); _s_err != nil { ... }`.
- `CallInfo.Results`, `ResultTypes` and `HasError` drive the shape.
- `or` supports at most one non-error result; multi-result calls cannot be
  used as a single expression.

### Multiple results

- `FuncDecl.Results`, `FuncType.Results`, `IFaceMethod.Results`,
  `FuncLit.Results` hold non-error results; `Err` adds the trailing error.
- Declarations use `fn f() A, B or error`; function types group with
  `fn(A) (B, C)`.
- `var a, b = f()` and `a, b = f()` are valid; `CallInfo.ResultTypes` lets sema
  type the names. A multi-result call is not a tuple value.

### Enums

```go
type Shape interface{ _s_shape() }

type ShapeCircle struct{ Radius float64 }
func (ShapeCircle) _s_shape() {}

type ShapeRect struct{ W, H float64 }
func (ShapeRect) _s_shape() {}
```

- Construction `Shape.Circle { radius = 2.0 }` → `&ShapeCircle{Radius: 2.0}`.
- `match` → type switch over `*ShapeCircle` etc.
- Variant field bindings become locals in the case clause.
- Exported variant types use the same case as the variant name.

### Structs

- Methods: `func (self *T) Name(args) ...`.
- Bare field identifiers resolved with `RefField` are emitted as `self.Field`.
- Struct literal: `T{Field: value}`.

### `par`

```go
var _s_wg sync.WaitGroup
var _s_pe _s_rt.ParErrors
_s_ctx, _s_cancel := context.WithCancel(_s_ctx)
_s_wg.Add(N)
go func() {
    defer _s_wg.Done()
    defer func() {
        if r := recover(); r != nil {
            _s_pe.Add(ord, _s_rt.NewPanicError(r))
            _s_cancel()
        }
    }()
    var _s_d _s_rt.DeferStack
    defer _s_d.RunTo(0)
    ...
}()
_s_wg.Wait()
if _s_err := _s_pe.First(); _s_err != nil {
    _s_cancel()
    return zero, _s_err
}
_s_cancel()
```

- Each top-level statement is its own goroutine and its own scope; `ord` is
  its index. A local declared in one worker is not visible in another.
- A cancelable context is created even when the body cannot fail statically: a
  panic in a callee must still cancel the siblings; it is cancelled at block
  exit.
- Each goroutine has its own `DeferStack` and runs it to zero at worker exit,
  before reporting an error or panic.
- `or return` / `or panic` drain the worker's `DeferStack` first, then add at
  the worker's ord, cancel and return from the goroutine. `or panic` adds an
  error (`AsError`), not a Go panic; a panic in a deferred call wins.
- A recovered panic is added as `*PanicError`; `Wait` completes, then
  `First()` decides.
- An error-returning function returns the winner; a `void` function re-panics
  it at the `par` statement.
- Writes to globals are rejected in both `par` and `par for`: a global is
  shared with the whole program, not only with the other workers.

### `par for`

- Sema has already rejected `return`/`break`/`continue` that leave the body.
- `limit N` default `runtime.GOMAXPROCS(0)`.
- Each iteration runs in its own goroutine with its own `DeferStack` and the
  same recovery boundary as `par`.
- Iteration variables are captured per iteration. A C-style init with
  assignment (`par for (i = 0; ...)`) captures `_s_iN := i` per iteration and
  rewrites body references to the copy; the condition and post use the outer
  variable. A direct write to `i` in the body is rejected.
- Errors are stored by iteration ordinal; lowest index wins. Map ranges use
  ord `-1` and first-completed semantics, including aliases and Go map types.
- `for (var x in xs)` uses the range index as ordinal.
- Map iterations report ord `-1`: every iteration is unordered, so the first
  error that reaches `ParErrors` wins. `First()` prefers any non-negative
  index before falling back to the earliest `-1` entry.

### `within`

```go
_s_ctx2, _s_cancel := context.WithTimeout(_s_ctx, d)
_s_d.Push(_s_cancel)
```

The block's calls use `_s_ctx2`; the mark/run mechanism cancels at exit. Inside
a worker `_s_ctx` is the `par` cancel context and `_s_d` is the worker's stack,
so one worker's deadline does not change a sibling's context; cancelling the
shared `par` context still cancels every worker's `within`.

### Tests

- `test "name" { ... };` → `func TestName(t *testing.T)` in
  `zz_strizh_test.go`, with unique suffix on duplicates.
- `assert(cond)` → `runtime.Assert(cond, "file:line")`.
- `assert_eq(a, b)` → `runtime.AssertEq(a, b, "file:line")`.

## Deep recursion

Strizh does not guarantee tail calls. Deep recursion can overflow the Go stack
the same way Go code does.

## Generated runtime

`runtime/` is a normal Go package that is embedded into the compiler and copied
to `.strizh/runtime` in the target project.

Required API:

```go
type DeferStack struct{ ... }
func (d *DeferStack) Mark() int
func (d *DeferStack) Push(fn func())
func (d *DeferStack) RunTo(mark int)
func Assert(ok bool, pos string)
func AssertEq[T comparable](got, want T, pos string)
func Has[K comparable, V any](m map[K]V, k K) bool

type PanicError struct {
    Value any
    Stack []byte
}
func NewPanicError(v any) *PanicError
func (e *PanicError) Error() string
func AsError(v any) error

type ParErrors struct{ ... }
func (p *ParErrors) Add(index int, err error)
func (p *ParErrors) First() error
```

`ParErrors.First()` returns the lowest non-negative index. Entries with `-1`
(map iterations) are considered only when no non-negative entry exists, and
then the earliest `Add` wins.
