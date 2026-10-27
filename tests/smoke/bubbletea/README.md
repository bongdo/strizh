# Bubble Tea smoke

Compile-only smoke for `charm.land/bubbletea/v2`. It checks `tea.Model`,
multiple results, `case T as name`, bare `nil`, and the G10 method wrapper
(`Update` uses `par`).

```sh
strizh build
```

Not part of `bin/check`: it downloads Bubble Tea and builds a real TUI.
