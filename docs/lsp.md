# Editor setup

The server is `strizh lsp`. It speaks LSP over stdio and uses the project root
with `strizh.toml` (or the current directory).

## Neovim

Add to `init.lua`:

```lua
vim.filetype.add({ extension = { str = "strizh" } })

vim.api.nvim_create_autocmd("FileType", {
  pattern = "strizh",
  callback = function(args)
    vim.lsp.start({
      name = "strizh",
      cmd = { "strizh", "lsp" },
      root_dir = vim.fs.root(args.buf, "strizh.toml"),
    })
  end,
})
```

Format on save:

```lua
vim.api.nvim_create_autocmd("BufWritePre", {
  pattern = "*.str",
  callback = function()
    vim.lsp.buf.format({ name = "strizh" })
  end,
})
```

## VS Code

```sh
cd editors/vscode
npm install
npm run compile
```

Open the folder in VS Code and press F5 (Run Extension). The grammar is
`source.strizh`, files end in `.str`. `strizh.serverPath` points at the
executable if it is not on `PATH`.

## Features

- diagnostics from parsing and Strizh checks
- basic hover with types and signatures
- go-to-definition for local, package and cross-file symbols where the symbol is resolved
- basic completion for keywords, builtins, locals and package members; struct fields and Go selectors are limited
- document symbols
- signature help
- document formatting

Go type errors are reported by `strizh build` / `strizh check`; the editor
shows the Strizh-specific diagnostics immediately. Generated Go contains
`//line` directives, so compiler errors point back to `.str` files.
