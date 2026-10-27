# Strizh for VS Code

- syntax highlighting (`source.strizh`)
- LSP client: diagnostics and basic editing support (hover, definition, completion, symbols, formatting);
  signature help

## Build

```sh
npm install
npm run compile
```

Open this folder in VS Code and press F5. The extension starts `strizh lsp`
from `PATH`; set `strizh.serverPath` if it lives elsewhere.
