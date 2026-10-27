package format

import (
	"strings"
	"testing"

	"github.com/bongdo/strizh/internal/parser"
)

const sample = `use fmt;
use "./lib" as lib;

// user type
type User = struct {
    Name: str,
    age: u32,

    fn Greet() str = {
        return "hi, " + Name;
    };
};

type Shape = enum {
    Circle { radius: f64 },
    Empty,
};

type Writer = interface {
    Write(p: u8[]) int or error;
};

type Id = u64;

const Max = 10;

fn area(s: Shape) f64 = {
    return match (s) {
    case Circle { radius } => 3.14 * radius * radius;
    case Empty => 0.0;
    };
};

fn read(path: str) u8[] or error = {
    var f = lib.Open(path) or return;
    defer f.Close();
    var buf = u8[16] {};
    var n = f.Read(buf[:]) or return;
    return buf[:n];
};

fn main() void or error = {
    var u = User { Name = "a", age = 1 };
    var s = u.Greet();
    if (len(s) > 0) {
        fmt.Println(s) or return;
    } else {
        fmt.Println("empty") or return;
    };
    for (var i = 0; i < 10; i += 1) {
        if (i % 2 == 0) { continue; };
    };
    par {
        fmt.Println("a") or return;
        fmt.Println("b") or return;
    };
    within 10s {
        fmt.Println("c") or return;
    };
    var xs = u8[] {1, 2};
    var m = map[str, i32] { "a" = 1 };
    for (var k, v in m) {
        fmt.Println(k, v) or return;
    };
    var f = fn(x: i32) i32 => x + 1;
    var g = fn(x: i32) i32 { return x + 1; };
    var n = len(xs);
    var p = nil[*User];
    _ = p;
    _ = f;
    _ = g;
    _ = n;
    return;
};

test "works" {
    assert(max(1, 2) == 2);
};
`

func TestRoundTrip(t *testing.T) {
	f, errs := parser.Parse("sample.str", []byte(sample))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out := File(f)
	f2, errs := parser.Parse("sample.str", out)
	if len(errs) > 0 {
		t.Fatalf("parse formatted: %v\n---\n%s", errs, out)
	}
	out2 := File(f2)
	if string(out) != string(out2) {
		t.Fatalf("not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out, out2)
	}
}

func TestNewSyntax(t *testing.T) {
	src := `fn pair() i32, i32 or error = {
    return 1, 2;
};

type Handler = fn(i32) (str, bool);

fn main() void = {
    var x = nil;
    match (msg) {
    case tea.KeyPressMsg as k => {};
    else => {};
    };
};

`
	f, errs := parser.Parse("new.str", []byte(src))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out := string(File(f))
	for _, want := range []string{"fn pair() i32, i32 or error", "fn(i32) (str, bool)", "var x = nil;", "case tea.KeyPressMsg as k", "else =>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
