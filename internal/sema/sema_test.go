package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/parser"
)

func checkSrc(t *testing.T, files map[string]string) *Program {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed := make(map[string][]*ast.File)
	for rel, src := range files {
		dir := root
		if rel != "." {
			dir = filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		path := filepath.Join(dir, "main.str")
		f, errs := parser.Parse(path, []byte(src))
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", rel, errs)
		}
		parsed[rel] = append(parsed[rel], f)
	}
	return Check(parsed, Options{Root: root, Module: "example.com/test"})
}

func noErrors(t *testing.T, p *Program) {
	t.Helper()
	if len(p.Errors) == 0 {
		return
	}
	var b strings.Builder
	for _, e := range p.Errors {
		b.WriteString(e.String())
		b.WriteByte('\n')
	}
	t.Fatalf("unexpected sema errors:\n%s", b.String())
}

func TestHello(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `use fmt;

fn main() void or error = {
    fmt.Println("hello") or return;
};
`})
	noErrors(t, p)
}

func TestUseForms(t *testing.T) {
	p := checkSrc(t, map[string]string{
		".": `use fmt as f;
use "net/http" as http;
use "./lib" as lib;

fn main() void or error = {
    f.Println("hi") or return;
    var h = http.Header {};
    _ = h;
    _ = lib.Square(2);
};
`,
		"lib": `fn Square(x: i32) i32 = {
    return x * x;
};
`,
	})
	noErrors(t, p)
}

func TestStructMethod(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `type User = struct {
    Name: str,
    age: u32,

    fn Greet() str = {
        return "hi, " + Name;
    };

    fn Rename(name: str) void = {
        Name = name;
    };
};

fn main() void = {
    var u = User { Name = "a", age = 3 };
    var s = u.Greet();
    u.Rename(s);
};
`})
	noErrors(t, p)
}

func TestStructLiteralNeedsNamedFields(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `type User = struct {
    Name: str,
};

fn main() void = {
    var u = User { "a" };
    _ = u;
};
`})
	wantError(t, p, "struct literal entry needs field = value")
}

func TestEnumMatch(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `type Shape = enum {
    Circle { radius: f64 },
    Rect { w: f64, h: f64 },
    Empty,
};

fn area(s: Shape) f64 = {
    return match (s) {
    case Circle { radius } => 3.14 * radius * radius;
    case Rect { w, h } => w * h;
    case Empty => 0.0;
    };
};

fn main() void = {
    var s = Shape.Circle { radius = 2.0 };
    var a = area(s);
    match (s) {
    case Circle { radius } => a = radius;
    case Rect { w, h } => a = w * h;
    case Empty => a = 0.0;
    };
};
`})
	noErrors(t, p)
}

func TestOrAndDefer(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `use os;
use fmt;

fn read(path: str) str or error = {
    var f = os.Open(path) or return;
    defer f.Close();
    var buf = u8[16] {};
    var n = f.Read(buf[:]) or return;
    _ = n;
    fmt.Println(path) or return;
    return path;
};

fn main() void or error = {
    var s = read("x") or err {
        fmt.Println("failed", err) or panic("output");
        return err;
    };
    fmt.Println(s) or return;
};
`})
	noErrors(t, p)
}

func TestGeneric(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `type Stack[Item] = struct {
    data: Item[],

    fn Push(x: Item) void = {
        data = append(data, x);
    };

    fn Len() int = {
        return len(data);
    };
};

fn Map[In, Out](xs: In[], f: fn(In) Out) Out[] = {
    var out = Out[] {};
    for (var x in xs) {
        out = append(out, f(x));
    };
    return out;
};

fn main() void = {
    var s = Stack[i32] {};
    s.Push(1);
    var n = s.Len();
    var ys = Map[i32, i32](i32[] {1, 2}, fn(x: i32) i32 => x * 2);
};
`})
	noErrors(t, p)
}

func TestControl(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `fn main() void = {
    var total = 0;
    for (var i = 0; i < 10; i += 1) {
        if (i == 3) { continue; };
        if (i == 8) { break; };
        total += i;
    };
    var xs = i32[] {1, 2, 3};
    for (var i, x in xs) {
        total += i + x;
    };
    var m = map[str, i32] { "a" = 1 };
    for (var k, v in m) {
        total += v + len(k);
    };
};
`})
	noErrors(t, p)
}

func TestPar(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `fn load(x: i32) i32 or error = {
    return x;
};

fn main() void or error = {
    var a = 0;
    var b = 0;
    par {
        a = load(1) or return;
        b = load(2) or return;
    };
    var xs = i32[] {1, 2, 3};
    var out = i32[3] {};
    par for (var i = 0; i < 3; i += 1) limit 2 {
        out[i] = xs[i];
    };
    within 10s {
        a = load(3) or return;
    };
};
`})
	noErrors(t, p)
}

func TestLocalPackage(t *testing.T) {
	p := checkSrc(t, map[string]string{
		".": `use "./lib" as lib;

fn main() void = {
    var n = lib.Square(3);
};
`,
		"lib": `fn Square(x: i32) i32 = {
    return x * x;
};
`,
	})
	noErrors(t, p)
}

func TestParWriteRejected(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `fn main() void = {
    var total = 0;
    par for (var i = 0; i < 4; i += 1) {
        total += i;
    };
};
`})
	if len(p.Errors) == 0 {
		t.Fatal("expected a par write error")
	}
}
