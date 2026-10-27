# Type-checking generated Go (G26)

Status: prototype works for `strizh check` and LSP diagnostics. It is not
async and does not check test packages.

## What runs

`internal/typecheck.Check` takes the checked `sema.Program`, generates Go in
memory with `internal/gen` (same generator as the build), and loads the result
with `golang.org/x/tools/go/packages`. Nothing is written to the source tree:

- every generated `zz_strizh.go` path is passed to `packages.Config.Overlay`;
- the embedded runtime is overlaid at `<module>/.strizh/runtime/runtime.go`;
- the packages are loaded from the real module root, so `go.mod`, `go.sum`
  and the module cache resolve imports normally;
- `//line /abs/path/file.str:line` directives already emitted by codegen make
  the Go compiler report positions in `.str` files. `typecheck` parses those
  positions and returns `Error{Path, Pos, Msg}`.

The load mode is `LoadImports | NeedExportFile`, not `LoadTypes`: requesting
`go/types` objects makes `go/packages` fall back to source-typing every
dependency because an overlay invalidates export data. Asking for export files
instead runs the Go compiler on the generated package (`go list -export`) and
returns its errors, which is the same type oracle and roughly 4-8x faster on
the projects measured here.

`strizh check` calls it after a clean parse/sema pass:

```sh
strizh check   # now exits 1 on `var n = 1 + "x";`
```

`strizh build/run/test` still rely on `go build` for type errors, so they do not
pay for a second type-check.

## Cache

Results are cached in memory, keyed by SHA-256 of:

- every generated Go file (path and contents, including `//line` paths);
- `go.mod` and `go.sum`;
- the embedded runtime source.

Identical input returns the previous diagnostics without invoking `go list`.
The cache holds the last 16 results. Infrastructure failures (missing module,
network, go list errors that cannot be mapped to a `.str` file) are not cached.
A local `replace` target that changes in place is not part of the key: restart
the LSP after editing one.

## LSP

`internal/lsp` runs the same check and publishes mapped diagnostics with
source `strizh`:

- `initialize`, `didOpen`, `didSave`, `didClose` run it;
- `didChange` does not, so typing does not invoke `go list`; type diagnostics
  are cleared until the next save;
- if parsing or sema has a non-warning error, type-checking is skipped;
- projects without `go.mod` are skipped, because packages cannot be resolved
  reliably.

The call is synchronous, like the existing sema pass: a save may block the
message loop while `go list` runs. Measured on this machine, type-checking
adds about 1-2 s to a small project; projects with large Go dependencies take
longer, but most of that is the existing sema/go-list work. A warm in-process
cache makes repeated opens and saves free.

## Known limits

- No columns: `//line` emits only `path:line`, so every error is reported at
  column 1. This is G34.
- Errors are only as good as Go's position mapping: an error in generated
  code that is not covered by a `//line` directive becomes a project-level
  error. If the package also has a `.str` parse/sema error, that unmapped
  project-level error can be dropped while the source error is reported.
- Test code (`zz_strizh_test.go`) is not type-checked; `strizh test` still
  finds its errors through `go test`.
- External Go imports must already be present in `go.mod`/`go.sum`. The LSP
  does not run `go mod tidy`.
- The cache key does not include dependency sources, hand-written `.go` files
  in the project, or `GOOS`/`GOARCH`. Restart the LSP after editing one of
  those (there is a `shortcut:` in `typecheck.go`).
- The LSP still does the whole-project sema pass on every keystroke (G43);
  only the Go type-check is restricted to save/open.

## Next steps

1. Run the check in a worker with a debounce (G43), keep the last published
   diagnostics, and coalesce saves.
2. Cache per package instead of per project, so editing one package does not
   reload unrelated ones.
3. Add columns to `//line` (G34) so diagnostics underline the expression.
4. Reuse the same diagnostics in `strizh test` for test files.

## Verification

- `internal/driver` test `TestCheckTypeErrors`: a type error in a local
  package is reported at `lib/lib.str:2`, and no `zz_strizh.go` is written to
  the source tree.
- `internal/lsp` test `TestTypeDiagnostics`: `int + "x"` is published on
  `didOpen`, cleared on `didChange`, and published again from cache on
  `didSave`.
- Manual: all `tests/cases` still pass `strizh check` with the new
  type-checking enabled.
