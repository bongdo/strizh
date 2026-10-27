package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/parser"
	"github.com/bongdo/strizh/internal/sema"
	"github.com/bongdo/strizh/internal/token"
	strizhrt "github.com/bongdo/strizh/runtime"
)

type project struct {
	dir    string
	prog   *sema.Program
	files  []File
	srcs   map[string]string
	parsed map[string][]*ast.File
}

func newProject(t *testing.T, srcs map[string]string) *project {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed := make(map[string][]*ast.File)
	for name, src := range srcs {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		f, errs := parser.Parse(path, []byte(src))
		for _, e := range errs {
			t.Fatalf("%s:%d:%d: %s", name, e.Pos.Line, e.Pos.Col, e.Msg)
		}
		rel := filepath.ToSlash(filepath.Dir(name))
		if rel == "." {
			rel = "."
		}
		parsed[rel] = append(parsed[rel], f)
	}
	prog := sema.Check(parsed, sema.Options{Root: dir, Module: "example.com/p"})
	for _, e := range prog.Errors {
		t.Fatalf("sema: %s", e.String())
	}
	files, errs := Generate(prog, parsed, Options{Root: dir, Module: "example.com/p", IncludeTests: true})
	for _, e := range errs {
		t.Fatalf("gen: %v", e)
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.Path, f.Src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rt := filepath.Join(dir, ".strizh", "runtime")
	if err := os.MkdirAll(rt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "runtime.go"), strizhrt.Source, 0o644); err != nil {
		t.Fatal(err)
	}
	return &project{dir: dir, prog: prog, files: files, srcs: srcs, parsed: parsed}
}

func (p *project) source(t *testing.T, name string) string {
	t.Helper()
	want := filepath.Join(p.dir, filepath.FromSlash(name))
	for _, f := range p.files {
		if f.Path == want {
			return string(f.Src)
		}
	}
	t.Fatalf("generated file %s not found", name)
	return ""
}

func (p *project) allSource() string {
	var b strings.Builder
	for _, f := range p.files {
		b.Write(f.Src)
	}
	return b.String()
}

func goCommand(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (p *project) build(t *testing.T) string {
	t.Helper()
	out, err := goCommand(t, p.dir, "build", "-o", filepath.Join(p.dir, "bin", "app"), ".")
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return filepath.Join(p.dir, "bin", "app")
}

func (p *project) run(t *testing.T) (string, string, error) {
	t.Helper()
	bin := p.build(t)
	cmd := exec.Command(bin)
	cmd.Dir = p.dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (p *project) test(t *testing.T) string {
	t.Helper()
	out, err := goCommand(t, p.dir, "test", "./...")
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
	return out
}

func expect(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func contains(t *testing.T, src, want string) {
	t.Helper()
	if !strings.Contains(src, want) {
		t.Fatalf("generated source does not contain %q:\n%s", want, src)
	}
}

func TestTypeStr(t *testing.T) {
	g := &generator{opt: Options{Module: "example.com/p"}, p: &sema.Program{Module: "example.com/p"}}
	f := &fileGen{g: g, pkg: &sema.Package{Imports: map[string]*sema.Import{"ed": {Alias: "ed", Path: "example.com/p/ed"}}}, need: map[string]bool{}, used: map[string]bool{}}
	enum := &ast.TypeDecl{Name: "Shape", GoName: "Shape"}
	variant := &ast.Variant{Name: "Circle", GoName: "ShapeCircle", Enum: enum, Fields: []*ast.Field{{Name: "Radius", Type: &ast.NamedType{Name: "f64"}}}}
	named := func(name string) ast.Type { return &ast.NamedType{Name: name, GoName: name} }
	cases := []struct {
		typ  ast.Type
		want string
	}{
		{named("str"), "string"},
		{named("u8"), "uint8"},
		{named("i64"), "int64"},
		{named("usize"), "uint"},
		{named("isize"), "int"},
		{named("f32"), "float32"},
		{named("rune"), "rune"},
		{named("byte"), "byte"},
		{named("any"), "any"},
		{named("error"), "error"},
		{named("void"), ""},
		{named("Duration"), "_s_time.Duration"},
		{named("Type"), "_s_reflect.Type"},
		{&ast.SliceType{Elem: named("u8")}, "[]uint8"},
		{&ast.ArrayType{Elem: named("u8"), Len: &ast.BasicLit{Kind: token.Int, Value: "3"}}, "[3]uint8"},
		{&ast.MapType{Key: named("str"), Value: named("i32")}, "map[string]int32"},
		{&ast.PointerType{Elem: named("u8")}, "*uint8"},
		{&ast.GenericType{Base: &ast.NamedType{Name: "Stack", GoName: "Stack"}, Args: []ast.Type{named("i32")}}, "Stack[int32]"},
		{&ast.FuncType{Params: []ast.Type{named("str")}, Results: []ast.Type{named("i32")}, Err: true}, "func(string) (int32, error)"},
		{&ast.FuncType{Params: []ast.Type{named("str")}, Err: true}, "func(string) error"},
		{&ast.FuncType{NeedsCtx: true, Params: []ast.Type{named("str")}}, "func(_s_context.Context, string)"},
		{&ast.NamedType{Variant: variant, PkgAlias: "ed"}, "ed.ShapeCircle"},
		{&ast.NamedType{PkgAlias: "http", GoName: "Request", Name: "Request"}, "http.Request"},
		{&ast.NamedType{Package: "ed", Name: "User"}, "ed.User"},
		{&ast.InterfaceType{Methods: []*ast.IFaceMethod{{Name: "Write", Params: []*ast.Param{{Name: "p", Type: &ast.SliceType{Elem: named("u8")}}}, Results: []ast.Type{named("i32")}, Err: true}}}, "interface{ Write(p []uint8) (int32, error) }"},
		{&ast.StructType{Fields: []*ast.Field{{Name: "X", Type: named("i32")}}}, "struct{ X int32 }"},
		{&ast.GoType{Text: "any"}, "any"},
		{&ast.NamedType{Name: "str"}, "string"},
		{&ast.NamedType{Name: "T"}, "T"},
	}
	for _, c := range cases {
		if got := g.typeStr(f, c.typ); got != c.want {
			t.Errorf("typeStr = %q, want %q", got, c.want)
		}
	}
	for _, key := range []string{"time", "reflect", "context"} {
		if !f.need[key] {
			t.Errorf("missing stdlib need %s", key)
		}
	}
	if !f.used["ed"] || !f.used["http"] {
		t.Errorf("missing user imports: %v", f.used)
	}
}

func TestHello(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": "use fmt;\n\nfn main() void or error = {\n    fmt.Println(\"hello\") or return;\n};\n",
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "// Code generated by strizh; DO NOT EDIT.")
	contains(t, src, "_s_fmt \"fmt\"")
	contains(t, src, "_s_os \"os\"")
	contains(t, src, "func _s_main() error")
	contains(t, src, "func main()")
	contains(t, src, "_s_fmt.Fprintln(_s_os.Stderr, _s_err)")
	contains(t, src, "//line "+filepath.Join(p.dir, "main.str")+":3")
	if strings.Contains(src, "_s_rt") {
		t.Fatalf("unexpected runtime import:\n%s", src)
	}
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "hello\n")
}

func TestNoImports(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": "fn main() void = {\n};\n",
	})
	src := p.source(t, "zz_strizh.go")
	if strings.Contains(src, "import (") {
		t.Fatalf("unexpected import block:\n%s", src)
	}
	contains(t, src, "func main() {\n\t_s_main()\n}")
}

func TestStructsEnumsMatch(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

type User = struct {
    Name: str,
    age: i32,

    fn Greet() str = {
        return "hi " + self.Name;
    };

    fn setAge(n: i32) void = {
        age = n;
    };

    fn CtxAge() i32 = {
        within 5s {
            return age;
        };
    };

    fn shadow(self: i32) i32 = {
        return age + self;
    };

    fn localSelf() i32 = {
        var self = i32(5);
        return self + age;
    };

    fn bump() void = {
        age += 1;
    };

    fn show() void = {
        fmt.Println("age", age);
    };
};

type Shape = enum {
    Circle { radius: f64 },
    Rect { w: f64, h: f64 },
    Empty,
};

type Greeter = interface {
    Greet() str;
};

fn makeGreeter(u: *User) Greeter = {
    return Greeter(u);
};

fn area(s: Shape) f64 = {
    return match (s) {
    case Circle { radius } => 3.0 * radius * radius;
    case Rect { w, h } => w * h;
    case Empty => 0.0;
    };
};

fn describe(s: Shape) str = {
    match (s) {
    case Circle { radius } => {
        var tag = "circle";
        if (i32(radius) > 1) {
            tag = "big circle";
        };
        return tag;
    };
    case Rect { w, h } => {
        return "rect";
    };
    case Empty => {
        return "empty";
    };
    };
    return "?";
};

fn main() void = {
    var u = User { Name = "Ann", age = 1 };
    u.setAge(30);
    var c = Shape.Circle { radius = 2.0 };
    var r = Shape.Rect { w = 2.0, h = 3.0 };
    var e = Shape.Empty;
    var g = makeGreeter(&u);
    defer u.show();
    defer u.bump();
    defer u.CtxAge();
    fmt.Println(u.Greet(), u.age, area(c), area(r), area(e), describe(c), describe(r), describe(e), g.Greet(), u.CtxAge(), u.shadow(2), u.localSelf());
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "type Shape interface{ _s_isShape() }")
	contains(t, src, "type ShapeCircle struct")
	contains(t, src, "func (*ShapeCircle) _s_isShape() {}")
	contains(t, src, "Shape(&ShapeCircle{radius:")
	contains(t, src, "Shape(&ShapeEmpty{})")
	contains(t, src, "switch _s_m")
	contains(t, src, "case *ShapeCircle:")
	contains(t, src, "self.Name")
	contains(t, src, "self.age = n")
	contains(t, src, "type Greeter interface")
	contains(t, src, "Greeter(u)")
	contains(t, src, "func (self *User) CtxAge() int32")
	contains(t, src, "func _s_User_CtxAge_ctx(self *User, _s_ctx _s_context.Context) int32")
	contains(t, src, "func (_s_self *User) shadow(self int32) int32")
	contains(t, src, "func (_s_self *User) localSelf() int32")
	contains(t, src, "_s_ctx := _s_context.Background()")
	contains(t, src, "_s_d.Push(func() {")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "hi Ann 30 12 6 0 big circle rect empty hi Ann 30 32 35\nage 31\n")
}

func TestGenerics(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

type Pair[A, B] = struct {
    First: A,
    Second: B,
};

type Stack[Item] = struct {
    data: Item[],

    fn Push(x: Item) void = {
        data = append(data, x);
    };

    fn Len() int = {
        return len(data);
    };
};

type Maybe[T] = T[];

fn MakePair[A, B](a: A, b: B) Pair[A, B] = {
    return Pair[A, B] { First = a, Second = b };
};

fn First[Item](xs: Item[]) Item = {
    return xs[0];
};

fn main() void = {
    var p = MakePair(7, "x");
    var xs = i32[] {4, 5};
    var ms = Maybe[i32] {8, 9};
    fmt.Println(p.First, p.Second, First(xs), First[i32](xs), ms[0], ms[1]);
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "type Pair[A any, B any] struct")
	contains(t, src, "type Stack[Item any] struct")
	contains(t, src, "func (self *Stack[Item]) Push(x Item)")
	contains(t, src, "func (self *Stack[Item]) Len() int")
	contains(t, src, "type Maybe[T any] = []T")
	contains(t, src, "func MakePair[A any, B any](a A, b B) Pair[A, B]")
	contains(t, src, "func First[Item any](xs []Item) Item")
	contains(t, src, "First[int32](xs)")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "7 x 4 4 8 9\n")
}

func TestErrorsAndDefer(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use errors;

fn fetch(n: i32) i32 or error = {
    if (n < 0) {
        return 0, errors.New("negative");
    };
    return n * 2;
};

fn guard(n: i32) void or error = {
    if (n == 0) {
        return errors.New("zero");
    };
};

fn pass(n: i32) i32 or error = {
    return fetch(n);
};

fn passthru(n: i32) void or error = {
    return guard(n);
};

fn two() i32 or error = {
    return 4, nil[error];
};

fn boom() void = {
    var x = fetch(-1) or panic("bad");
    var y = fetch(-1) or panic();
    fmt.Println(x, y);
};

fn main() void or error = {
    var a = fetch(3) or return;
    var b = fetch(-1) or err {
        fmt.Println("got", err) or return;
        var err = 99;
        fmt.Println("shadow", err) or return;
    };
    fmt.Println(a, b) or return;
    guard(0) or err {
        fmt.Println("guard:", err) or return;
    };
    var v, e = two();
    if (e == nil[error]) {
        fmt.Println("two", v) or return;
    };
    var c = 0;
    {
        defer fmt.Println("block");
        c = 1;
    };
    defer fmt.Println("func");
    fmt.Println(c) or return;
    var square = fn(x: i32) i32 {
        defer fmt.Println("lambda");
        return x * x;
    };
    fmt.Println(square(3)) or return;
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "_s_v")
	contains(t, src, "_s_err")
	contains(t, src, "return _s_err")
	contains(t, src, "panic(\"bad\")")
	contains(t, src, "panic(_s_err")
	contains(t, src, "return fetch(n)")
	contains(t, src, "return guard(n)")
	contains(t, src, "var v, e = two()")
	contains(t, src, "return 4, (error)(nil)")
	contains(t, src, "_s_d.Push(func() {")
	contains(t, src, "var _s_d _s_rt.DeferStack")
	contains(t, src, "defer _s_d.RunTo(0)")
	contains(t, src, "_s_d.RunTo(_s_mark")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "got negative\nshadow 99\n6 0\nguard: zero\ntwo 4\nblock\n1\nlambda\n9\nfunc\n")
}

func TestDeferLoopCleanup(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

fn main() void = {
    for (var i = 0; i < 3; i += 1) {
        defer fmt.Println("iter", i);
        if (i == 1) {
            continue;
        };
        if (i == 2) {
            break;
        };
        fmt.Println("body", i);
    };
    fmt.Println("done");
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "body 0\niter 0\niter 1\niter 2\ndone\n")
}

func TestControlFlow(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

const N = 2;
var counter = i32(5);

fn shadowParam(x: i32) i32 = {
    var x = i32(3);
    return x;
};

fn main() void = {
    var k1, k2 = 1, 2;
    var arr = i32[N] {7, 8};
    var xs = i32[] {10, 20, 30};
    var sum = i32(0);
    for (var x in xs) {
        sum += x;
    };
    var m = map[str, i32] { "a" = 1, "b" = 2 };
    var keys = str[] {};
    var total = i32(0);
    for (var k, v in m) {
        keys = append(keys, k);
        total += v;
    };
    var seen = 0;
    for (var _ in m) {
        seen += 1;
    };
    var n = 0;
    for (var j = 0; j < 5; j += 1) {
        if (j == 2) {
            continue;
        };
        n += 1;
    };
    var buf = u8[] {};
    buf = append(buf, 65);
    var copyBuf = u8[] {0};
    copy(copyBuf, buf);
    var s = "hello";
    var zero = nil[error];
    var ys = i32[] {};
    var more = i32[] {1, 2};
    ys = append(ys, more...);
    var rs = runes("ab");
    var bs = bytes("hi");
    var shadow = fn(x: i32) i32 {
        var x = i32(4);
        return x;
    };
    var branch = "";
    if (n == 0) {
        branch = "zero";
    } else if (n == 4) {
        branch = "four";
    } else {
        branch = "other";
    };
    var once = 0;
    for () {
        once = 1;
        break;
    };
    delete(m, "b");
    fmt.Println(sum, len(xs), cap(xs), total, len(keys), seen, n, str(copyBuf[0]), s[1:3], has(m, "a"), min(3, 5), max(3, 5), zero == nil[error], len(ys), counter, N, k1 + k2, len(arr), branch, once, len(xs[:2]), len(xs[1:]), len(xs[:]), has(m, "b"), len(rs), rs[0], len(bs), str(rs), shadowParam(9), shadow(1));
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "for _, x := range xs")
	contains(t, src, "for k, v := range m")
	contains(t, src, "for range m")
	contains(t, src, "(error)(nil)")
	contains(t, src, "[]rune(\"ab\")")
	contains(t, src, "[]byte(\"hi\")")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "60 3 3 3 2 2 4 A el true 3 5 true 2 5 2 3 2 four 1 2 2 3 false 2 97 2 ab 3 4\n")
}

func TestParWithin(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

fn work(n: int) int = {
    return n * 10;
};

fn main() void or error = {
    var a = 0;
    var b = 0;
    par {
        a = work(1);
        b = work(2);
    };
    var slots = int[4] {};
    par for (var i = 0; i < 4; i += 1) limit 2 {
        slots[i] = work(i);
    };
    var c = 0;
    within 5s {
        c = work(3);
    };
    var nums = int[] {1, 2, 3};
    var out = int[3] {};
    par for (var idx, x in nums) {
        out[idx] = work(x);
    };
    var mm = map[int, int] { 1 = 4 };
    var got = int[2] {};
    par for (var mk, mv in mm) {
        if (mk == 1) {
            got[mk] = mv;
        };
    };
    fmt.Println(a + b, slots[0], slots[1], slots[2], slots[3], c, out[0], out[1], out[2], got[1]) or return;
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "_s_sync.WaitGroup")
	contains(t, src, "make(chan struct{}, 2)")
	contains(t, src, "_s_context.WithTimeout")
	contains(t, src, "go func() {")
	contains(t, src, "_s_i")
	contains(t, src, "_s_ctx := _s_context.Background()")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "30 0 10 20 30 30 10 20 30 4\n")
}

func TestParErrors(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use errors;

fn maybe(n: int) int or error = {
    if (n == 2) {
        return 0, errors.New("two");
    };
    return n;
};

fn main() void or error = {
    var p1 = 0;
    var p2 = 0;
    par {
        p1 = maybe(1) or return;
        p2 = maybe(3) or return;
    };
    fmt.Println(p1, p2) or return;
    var slots = int[3] {};
    par for (var i = 0; i < 3; i += 1) {
        slots[i] = maybe(i) or return;
    };
    fmt.Println(slots[0], slots[1], slots[2]) or return;
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "var _s_pe")
	contains(t, src, "_s_context.WithCancel")
	contains(t, src, ".Add(0, ")
	contains(t, src, ".Add(1, ")
	contains(t, src, ".First()")
	stdout, stderr, err := p.run(t)
	if err == nil {
		t.Fatalf("expected failure, stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "two") {
		t.Fatalf("stderr does not contain error: %s", stderr)
	}
	expect(t, stdout, "1 3\n")
}

func TestParPanic(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use errors;

fn boom() void = {
    panic("boom");
};

fn fail() void or error = {
    return errors.New("fail");
};

fn runPanic() void or error = {
    par {
        boom();
    };
};

fn runOrPanic() void or error = {
    par {
        fail() or panic("wrapped");
    };
};

fn main() void = {
    var p = false;
    var o = false;
    runPanic() or err { p = true; };
    runOrPanic() or err { o = true; };
    fmt.Println(p, o);
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "recover()")
	contains(t, src, "_s_rt.NewPanicError(r)")
	contains(t, src, "_s_rt.AsError(")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("panic reached stderr: %s", stderr)
	}
	expect(t, stdout, "true true\n")
}

func TestParForAssignInit(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

fn main() void = {
    var n = 8;
    var i = 0;
    var out = i32[8] {};
    par for (i = 0; i < n; i += 1) limit 4 {
        out[i] = i32(i * i);
    };
    fmt.Println(out, i);
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "[0 1 4 9 16 25 36 49] 8\n")
}

func TestParForMapError(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use errors;

fn maybe(n: i32) i32 or error = {
    if (n == 1) {
        return 0, errors.New("one");
    };
    return n;
};

fn run() void or error = {
    var m = map[i32, i32] { 1 = 1, 2 = 2 };
    var out = i32[3] {};
    par for (var k, v in m) limit 1 {
        out[k] = maybe(v) or return;
    };
};

fn main() void = {
    var caught = false;
    run() or err { caught = true; };
    fmt.Println(caught);
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, ".Add(-1, ")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "true\n")
}

func TestTests(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use strings;

fn add(a: i32, b: i32) i32 = {
    return a + b;
};

fn fail() void or error = {
    return;
};

fn tick(n: i32) i32 = {
    within 5s {
        return n + 1;
    };
};

fn main() void = {
};

test "add works" {
    assert_eq(add(2, 3), 5);
};

test "slice grows" {
    var xs = i32[] {};
    xs = append(xs, 7);
    assert_eq(len(xs), 1);
    assert(len(xs) == 1);
};

test "within works" {
    assert_eq(tick(4), 5);
};

test "imports and or return" {
    fail() or return;
    assert_eq(strings.ToUpper("a"), "A");
};
`,
	})
	src := p.source(t, "zz_strizh_test.go")
	contains(t, src, "_s_testing \"testing\"")
	contains(t, src, "func TestAddWorks(t *_s_testing.T)")
	contains(t, src, "func _s_testAddWorks() error")
	contains(t, src, "func _s_testWithinWorks(_s_ctx _s_context.Context) error")
	contains(t, src, "strings \"strings\"")
	contains(t, src, "t.Fatal(_s_err)")
	contains(t, src, "_s_rt.AssertEq")
	out := p.test(t)
	if !strings.Contains(out, "ok") {
		t.Fatalf("go test output:\n%s", out)
	}
	mainSrc := p.source(t, "zz_strizh.go")
	if strings.Contains(mainSrc, "func Test") {
		t.Fatalf("test function leaked into non-test file:\n%s", mainSrc)
	}
	if strings.Contains(mainSrc, "strings") {
		t.Fatalf("test-only import leaked into non-test file:\n%s", mainSrc)
	}
	files, errs := Generate(p.prog, p.parsed, Options{Root: p.dir, Module: "example.com/p"})
	for _, e := range errs {
		t.Fatal(e)
	}
	for _, f := range files {
		if strings.HasSuffix(f.Path, "_test.go") {
			t.Fatalf("test file generated without IncludeTests: %s", f.Path)
		}
	}
}

func TestLocalImport(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use "./internal/ed" as ed;
use fmt;

fn main() void = {
    var c = ed.MakeColor(true);
    var label = match (c) {
    case ed.Red => "red";
    case ed.Blue => "blue";
    };
    var c2 = ed.Color.Blue;
    var label2 = match (c2) {
    case ed.Red => "red";
    case ed.Blue => "blue";
    };
    var label3 = ed.Pick[ed.Color, str](c2, "blue");
    fmt.Println(ed.Double(21), ed.Ctx(), label, label2, label3);
};
`,
		"internal/ed/ed.str": `type Color = enum {
    Red,
    Blue,
};

fn MakeColor(red: bool) Color = {
    if (red) {
        return Color.Red;
    };
    return Color.Blue;
};

fn Pick[L, R](l: L, r: R) R = {
    return r;
};

fn Double(n: i32) i32 = {
    return n * 2;
};

fn Ctx() i32 = {
    within 5s {
        return 7;
    };
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "ed \"example.com/p/internal/ed\"")
	contains(t, src, "ed.Ctx()")
	contains(t, src, "ed.Color(&ed.ColorBlue{})")
	contains(t, src, "case *ed.ColorRed:")
	contains(t, src, "ed.Pick[ed.Color, string](c2, \"blue\")")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "42 7 red blue blue\n")
}

func TestGoContext(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use context;
use fmt;

fn main() void = {
    var ctx, cancel = context.WithCancel();
    defer cancel();
    if (ctx != nil[context.Context]) {
        fmt.Println("ctx");
    };
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "context.WithCancel(_s_ctx)")
	contains(t, src, "func _s_main(_s_ctx _s_context.Context)")
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "ctx\n")
}

func TestNestedPar(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use errors;

fn maybe(n: i32) i32 or error = {
    if (n == 1) {
        return 0, errors.New("one");
    };
    return n;
};

fn main() void or error = {
    par {
        par {
            maybe(1) or return;
        };
        maybe(2);
    };
    fmt.Println("after") or return;
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, "_s_pe")
	contains(t, src, "_s_context.WithCancel")
	stdout, stderr, err := p.run(t)
	if err == nil {
		t.Fatalf("expected failure, stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "one") {
		t.Fatalf("stderr does not contain propagated inner error: %s", stderr)
	}
}
