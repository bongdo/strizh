# Getting started

## Create a project

```sh
strizh new hello
cd hello
```

`strizh new` writes:

```toml
name = "hello"
module = "example.com/hello"
go = "1.27"
```

and a `main.str`:

```hare
use fmt;

fn main() void or error = {
    fmt.Println("hello from hello") or return;
};
```

## Run and build

```sh
strizh run
strizh build
./bin/hello
```

`strizh build` performs three steps every time: parse and type-check the
Strizh sources, generate Go next to them, and invoke `go build`. Go modules
are created and tidied automatically; external Go imports are added with
`go mod tidy`.

## Add a local package

`lib/math.str`:

```hare
fn Square(x: i32) i32 = {
    return x * x;
};
```

`main.str`:

```hare
use fmt;
use "./lib" as lib;

fn main() void or error = {
    fmt.Println(lib.Square(7)) or return;
};
```

Only exported (capitalized) names are visible across packages, exactly as in
Go.

## Errors

```hare
use os;

fn exists(path: str) bool or error = {
    var f = os.Open(path) or return;
    defer f.Close();
    return true;
};
```

- `call() or return;` propagates.
- `call() or err { ... };` handles the error here.
- `call() or panic("message");` aborts.

## Tests

```hare
test "square" {
    assert_eq(2 * 2, 4);
    assert(true);
};
```

`strizh test` runs every `test` block with `go test`.

## Concurrency

```hare
fn load(id: i32) i32 or error = { ... };

fn main() void or error = {
    var users = 0;
    var orders = 0;
    par {
        users = load(1) or return;
        orders = load(2) or return;
    };

    var xs = i32[] {1, 2, 3};
    var out = i32[3] {};
    par for (var i = 0; i < len(xs); i += 1) limit 2 {
        out[i] = xs[i] * 2;
    };

    within 30s {
        out[0] = load(3) or return;
    };
};
```

Captured state may be read concurrently. Writes must go to a slot indexed by
the iteration variable or be the only writer of that variable inside `par`.
`par`/`within` errors cancel the remaining work.

## Formatting

```sh
strizh fmt -w .
```

Four-space indentation, one statement per line, mandatory semicolons.
