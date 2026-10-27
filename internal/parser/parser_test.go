package parser

import (
	"strings"
	"testing"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/token"
)

func mustParse(t *testing.T, src string) *ast.File {
	t.Helper()
	f, errs := Parse("test.str", []byte(src))
	if f == nil {
		t.Fatal("Parse returned nil file")
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected error at %d:%d: %s", e.Pos.Line, e.Pos.Col, e.Msg)
		}
		t.FailNow()
	}
	return f
}

func checkSpans(t *testing.T, f *ast.File) {
	t.Helper()
	ast.Inspect(f, func(n ast.Node) bool {
		start, end := n.Pos(), n.EndPos()
		if start.Line <= 0 || end.Line <= 0 {
			t.Errorf("%T: invalid line: start=%+v end=%+v", n, start, end)
			return true
		}
		if start.Col <= 0 || end.Col <= 0 {
			t.Errorf("%T: invalid column: start=%+v end=%+v", n, start, end)
		}
		if start.Offset < 0 || end.Offset < start.Offset || end.Offset > len(f.Src) {
			t.Errorf("%T: bad span %d..%d (src %d)", n, start.Offset, end.Offset, len(f.Src))
		}
		return true
	})
	for _, c := range f.Comments {
		if c.Pos().Line <= 0 || c.EndPos().Offset < c.Pos().Offset {
			t.Errorf("bad comment span %+v..%+v", c.Pos(), c.EndPos())
		}
	}
}

func declType(t *testing.T, f *ast.File, name string) *ast.TypeDecl {
	t.Helper()
	for _, d := range f.Decls {
		if td, ok := d.(*ast.TypeDecl); ok && td.Name == name {
			return td
		}
	}
	t.Fatalf("type %q not found", name)
	return nil
}

func declFunc(t *testing.T, f *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name == name {
			return fd
		}
	}
	t.Fatalf("func %q not found", name)
	return nil
}

func findVar(t *testing.T, f *ast.File, name string) *ast.VarDecl {
	t.Helper()
	var found *ast.VarDecl
	ast.Inspect(f, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if v, ok := n.(*ast.VarDecl); ok && len(v.Names) > 0 && v.Names[0] == name {
			found = v
		}
		return true
	})
	if found == nil {
		t.Fatalf("var %q not found", name)
	}
	return found
}

func singleValue(t *testing.T, v *ast.VarDecl) ast.Expr {
	t.Helper()
	if len(v.Values) != 1 {
		t.Fatalf("var %v has %d values, want 1", v.Names, len(v.Values))
	}
	return v.Values[0]
}

func countNodes(f *ast.File) map[string]int {
	m := map[string]int{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.Block:
			m["block"]++
		case *ast.DeclStmt:
			m["declstmt"]++
		case *ast.AssignStmt:
			m["assign"]++
		case *ast.ExprStmt:
			m["exprstmt"]++
		case *ast.IfStmt:
			m["if"]++
		case *ast.ForStmt:
			m["for"]++
		case *ast.ReturnStmt:
			m["return"]++
		case *ast.BreakStmt:
			m["break"]++
		case *ast.ContinueStmt:
			m["continue"]++
		case *ast.DeferStmt:
			m["defer"]++
		case *ast.ParStmt:
			m["par"]++
		case *ast.WithinStmt:
			m["within"]++
		case *ast.MatchExpr:
			m["match"]++
		case *ast.OrExpr:
			m["or"]++
		case *ast.FuncLit:
			m["funclit"]++
		case *ast.CompositeLit:
			m["composite"]++
		case *ast.IndexExpr:
			m["index"]++
		case *ast.SliceExpr:
			m["slice"]++
		case *ast.CallExpr:
			m["call"]++
		case *ast.UnaryExpr:
			m["unary"]++
		case *ast.BinaryExpr:
			m["binary"]++
		case *ast.SelectorExpr:
			m["selector"]++
		}
		return true
	})
	return m
}

const realisticSrc = `use fmt;
use "./ed" as ed;

const Max = 100;

type User = struct {
    Name: str,
    age: u32,

    fn Greet() str = {
        return "hi, " + Name;
    };

    fn Rename(n: str) void = {
        Name = n;
    };
};

type Shape = enum {
    Circle { radius: f64 },
    Rect { w: f64, h: f64 },
    Empty,
};

type Writer = interface {
    fn Write(p: u8[]) int or error;
    fn Flush() void;
};

type Id = u64;
type Handler = fn(Request) Response or error;

type Stack[Item] = struct {
    data: Item[],

    fn Push(x: Item) void = {
        data = append(data, x);
    };
};

fn Map[In, Out](xs: In[], f: fn(In) Out) Out[] = {
    var out = Out[] {};
    for (var x in xs) {
        out = append(out, f(x));
    };
    return out;
};

var global = User { Name = "Alice", age = 30 };
var mapping = map[str, User] {};

fn main() void or error = {
    fmt.Println(global.Name) or return;
};

test "append grows the slice" {
    var xs = u8[] {};
    xs = append(xs, 1);
    assert_eq(len(xs), 1);
};
`

func TestRealisticFile(t *testing.T) {
	f := mustParse(t, realisticSrc)
	checkSpans(t, f)

	if f.Path != "test.str" {
		t.Errorf("Path = %q", f.Path)
	}
	if string(f.Src) != realisticSrc {
		t.Error("Src not preserved")
	}
	if len(f.Uses) != 2 {
		t.Fatalf("uses = %d, want 2", len(f.Uses))
	}
	if u := f.Uses[0]; u.Path != "fmt" || u.Alias != "" || u.Local {
		t.Errorf("use[0] = %+v", u)
	}
	if u := f.Uses[1]; u.Path != "./ed" || u.Alias != "ed" || !u.Local {
		t.Errorf("use[1] = %+v", u)
	}
	if len(f.Decls) != 12 {
		t.Fatalf("decls = %d, want 12", len(f.Decls))
	}

	user := declType(t, f, "User")
	if user.Kind != ast.TypeStruct {
		t.Errorf("User kind = %v", user.Kind)
	}
	if len(user.Fields) != 2 || user.Fields[0].Name != "Name" || user.Fields[1].Name != "age" {
		t.Errorf("User fields = %+v", user.Fields)
	}
	if len(user.Methods) != 2 {
		t.Fatalf("User methods = %d", len(user.Methods))
	}
	if user.Methods[0].Name != "Greet" || user.Methods[0].Owner != user {
		t.Errorf("method Greet = %+v", user.Methods[0])
	}
	if user.Methods[0].Body == nil || len(user.Methods[0].Body.Stmts) != 1 {
		t.Errorf("Greet body = %+v", user.Methods[0].Body)
	}

	shape := declType(t, f, "Shape")
	if shape.Kind != ast.TypeEnum || len(shape.Variants) != 3 {
		t.Fatalf("Shape = %+v", shape)
	}
	if shape.Variants[0].Name != "Circle" || len(shape.Variants[0].Fields) != 1 {
		t.Errorf("Circle = %+v", shape.Variants[0])
	}
	if shape.Variants[2].Name != "Empty" || len(shape.Variants[2].Fields) != 0 {
		t.Errorf("Empty = %+v", shape.Variants[2])
	}
	if shape.Variants[0].Enum != shape {
		t.Error("variant Enum not set")
	}

	writer := declType(t, f, "Writer")
	if writer.Kind != ast.TypeInterface || len(writer.IFace) != 2 {
		t.Fatalf("Writer = %+v", writer)
	}
	if m := writer.IFace[0]; m.Name != "Write" || !m.Err || len(m.Params) != 1 || m.Params[0].Name != "p" {
		t.Errorf("Write = %+v", m)
	}
	if m := writer.IFace[1]; m.Name != "Flush" || m.Err || len(m.Results) == 0 {
		t.Errorf("Flush = %+v", m)
	}

	id := declType(t, f, "Id")
	if id.Kind != ast.TypeAlias {
		t.Errorf("Id kind = %v", id.Kind)
	}
	if nt, ok := id.Target.(*ast.NamedType); !ok || nt.Name != "u64" {
		t.Errorf("Id target = %#v", id.Target)
	}

	handler := declType(t, f, "Handler")
	ft, ok := handler.Target.(*ast.FuncType)
	if !ok || len(ft.Params) != 1 || !ft.Err {
		t.Fatalf("Handler target = %#v", handler.Target)
	}

	stack := declType(t, f, "Stack")
	if len(stack.TypeParams) != 1 || stack.TypeParams[0].Name != "Item" {
		t.Errorf("Stack params = %+v", stack.TypeParams)
	}
	if len(stack.Fields) != 1 || len(stack.Methods) != 1 {
		t.Errorf("Stack body = %+v", stack)
	}

	mapper := declFunc(t, f, "Map")
	if len(mapper.TypeParams) != 2 || mapper.TypeParams[1].Name != "Out" {
		t.Errorf("Map type params = %+v", mapper.TypeParams)
	}
	if len(mapper.Params) != 2 {
		t.Fatalf("Map params = %d", len(mapper.Params))
	}
	if _, ok := mapper.Results[0].(*ast.SliceType); !ok {
		t.Errorf("Map result = %#v", mapper.Results[0])
	}

	global := findVar(t, f, "global")
	if global.Const {
		t.Error("global marked const")
	}
	if max := findVar(t, f, "Max"); !max.Const {
		t.Error("Max not marked const")
	}
	cl, ok := singleValue(t, global).(*ast.CompositeLit)
	if !ok {
		t.Fatalf("global value = %#v", global.Values[0])
	}
	if nt, ok := cl.Type.(*ast.NamedType); !ok || nt.Name != "User" {
		t.Errorf("global type = %#v", cl.Type)
	}
	if len(cl.Entries) != 2 || cl.Entries[0].Key != "Name" {
		t.Errorf("global entries = %+v", cl.Entries)
	}

	mapping := findVar(t, f, "mapping")
	if _, ok := singleValue(t, mapping).(*ast.CompositeLit); !ok {
		t.Errorf("mapping value = %#v", mapping.Values[0])
	}

	main := declFunc(t, f, "main")
	if !main.Err || len(main.Results) == 0 || main.Body == nil {
		t.Errorf("main = %+v", main)
	}

	var test *ast.TestDecl
	for _, d := range f.Decls {
		if td, ok := d.(*ast.TestDecl); ok {
			test = td
		}
	}
	if test == nil || test.Name != "append grows the slice" || test.Body == nil {
		t.Errorf("test decl = %+v", test)
	}
}

func TestUseCanonical(t *testing.T) {
	for _, src := range []string{
		"use fmt;",
		"use fmt as f;",
		`use "net/http" as http;`,
		`use "./lib" as lib;`,
		`use "../x";`,
	} {
		f, errs := Parse("use.str", []byte(src))
		if len(errs) != 0 {
			t.Errorf("%q: unexpected errors: %v", src, errs)
		}
		if len(f.Uses) != 1 {
			t.Errorf("%q: uses = %+v", src, f.Uses)
		}
	}

	for _, c := range []struct{ src, hint string }{
		{`use "fmt";`, "use fmt;"},
		{`use "fmt" as f;`, "use fmt;"},
		{"use `os`;", "use os;"},
	} {
		_, errs := Parse("use.str", []byte(c.src))
		if len(errs) != 1 || !strings.Contains(errs[0].Msg, c.hint) {
			t.Errorf("%q: errors = %v", c.src, errs)
		}
	}
}

const statementsSrc = `fn f(n: int) void = {
    var a, b = 1, 2;
    const c = 3;
    a = a + 1;
    a += 1;
    a -= 1;
    a *= 2;
    a /= 2;
    a %= 2;
    a &= 1;
    a |= 1;
    a ^= 1;
    a <<= 1;
    a >>= 1;
    if (a > 0) {
        a = 0;
    } else if (a == 0) {
        a = 1;
    } else {
        a = 2;
    };
    for (var i = 0; i < n; i += 1) {
        a = i;
    };
    for (var x in xs) {
        a = x;
    };
    for (var i, x in xs) {
        a = i + x;
    };
    for (var k, v in m) {
        a = k;
    };
    for () {
        break;
    };
    par {
        a = 1;
    };
    par for (var i = 0; i < n; i += 1) limit 4 {
        a = i;
    };
    par for (var j in xs) limit n {
        continue;
    };
    within d {
        a = 1;
    };
    match (a) {
    case 0 => a = 1;
    else => { a = 2; };
    };
    defer f.Close();
    return;
};
`

func TestStatements(t *testing.T) {
	f := mustParse(t, statementsSrc)
	checkSpans(t, f)

	counts := countNodes(f)
	want := map[string]int{
		"if":       2,
		"for":      7,
		"par":      1,
		"within":   1,
		"match":    1,
		"defer":    1,
		"return":   1,
		"break":    1,
		"continue": 1,
		"declstmt": 4,
	}
	for k, n := range want {
		if counts[k] != n {
			t.Errorf("%s count = %d, want %d", k, counts[k], n)
		}
	}
	if counts["assign"] < 15 {
		t.Errorf("assign count = %d, want at least 15", counts["assign"])
	}

	var outer *ast.IfStmt
	var parFors []*ast.ForStmt
	var mapRange *ast.ForStmt
	var emptyFor *ast.ForStmt
	var matchStmt *ast.MatchExpr
	var within *ast.WithinStmt
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt:
			if outer == nil {
				outer = n
			}
		case *ast.ForStmt:
			if n.Par {
				parFors = append(parFors, n)
			}
			switch {
			case n.Range != nil && len(n.Range.Names) == 2 && n.Range.Names[0] == "k":
				mapRange = n
			case n.Init == nil && n.Cond == nil && n.Post == nil && n.Range == nil:
				emptyFor = n
			}
		case *ast.MatchExpr:
			matchStmt = n
		case *ast.WithinStmt:
			within = n
		}
		return true
	})

	if outer == nil {
		t.Fatal("no if statement")
	}
	inner, ok := outer.Else.(*ast.IfStmt)
	if !ok {
		t.Fatalf("else = %#v, want *ast.IfStmt", outer.Else)
	}
	if _, ok := inner.Else.(*ast.Block); !ok {
		t.Errorf("final else = %#v, want *ast.Block", inner.Else)
	}

	if len(parFors) != 2 {
		t.Fatalf("par for count = %d, want 2", len(parFors))
	}
	var parLimit, parRange *ast.ForStmt
	for _, n := range parFors {
		if n.Range == nil {
			parLimit = n
		} else {
			parRange = n
		}
	}
	if parLimit == nil {
		t.Fatal("no par for with limit")
	}
	if lit, ok := parLimit.Limit.(*ast.BasicLit); !ok || lit.Value != "4" {
		t.Errorf("par limit = %#v", parLimit.Limit)
	}
	if parRange == nil || len(parRange.Range.Names) != 1 || parRange.Range.Names[0] != "j" {
		t.Errorf("par range = %+v", parRange)
	} else if id, ok := parRange.Limit.(*ast.Ident); !ok || id.Name != "n" {
		t.Errorf("par range limit = %#v", parRange.Limit)
	}
	if mapRange == nil {
		t.Fatal("no map range")
	}
	if emptyFor == nil || emptyFor.Body == nil {
		t.Errorf("infinite for = %+v", emptyFor)
	}
	if within == nil {
		t.Fatal("no within")
	}
	if id, ok := within.Duration.(*ast.Ident); !ok || id.Name != "d" {
		t.Errorf("within duration = %#v", within.Duration)
	}

	if matchStmt == nil {
		t.Fatal("no match")
	}
	if matchStmt.IsExpr {
		t.Error("statement match marked as expression")
	}
	if len(matchStmt.Cases) != 2 {
		t.Fatalf("cases = %d", len(matchStmt.Cases))
	}
	if c := matchStmt.Cases[0]; c.Expr != nil || c.Block != nil || len(c.Body) != 1 {
		t.Errorf("case 0 = %+v", c)
	}
	if c := matchStmt.Cases[1]; !c.Pat.Wildcard || c.Block == nil {
		t.Errorf("case _ = %+v", c)
	}

	var matchExprStmt bool
	ast.Inspect(f, func(n ast.Node) bool {
		if es, ok := n.(*ast.ExprStmt); ok {
			if _, ok := es.X.(*ast.MatchExpr); ok {
				matchExprStmt = true
			}
		}
		return true
	})
	if !matchExprStmt {
		t.Error("match statement not wrapped in ExprStmt")
	}
}

const expressionsSrc = "fn e() void or error = {\n" +
	"    var p = 1 + 2 * 3 - 4;\n" +
	"    var q = a || b && c == d + e * f;\n" +
	"    var c1 = f(1, 2);\n" +
	"    var c2 = x.M(1);\n" +
	"    var c3 = u64(x);\n" +
	"    var c4 = Map[i32, str](xs, f);\n" +
	"    var c5 = xs[i];\n" +
	"    var c6 = xs[i](1);\n" +
	"    var c7 = xs[1:2];\n" +
	"    var c8 = xs[:2];\n" +
	"    var c9 = xs[1:];\n" +
	"    var s1 = User { Name = \"Alice\", age = 30 };\n" +
	"    var m1 = map[str, int] { \"a\" = 1, };\n" +
	"    var m2 = map[str, int] { key = 1 };\n" +
	"    var sl = u8[] {1, 2, 3};\n" +
	"    var sh = Shape.Circle { radius = 2.0 };\n" +
	"    var e1 = Shape.Empty {};\n" +
	"    var fl1 = fn(i, j) => i < j;\n" +
	"    var fl2 = fn(i: int, j: int) bool { return i < j; };\n" +
	"    var np = nil[*Node];\n" +
	"    var o1 = os.Open(\"x\") or return;\n" +
	"    var o2 = os.Open(\"x\") or err { return err; };\n" +
	"    var o3 = os.Open(\"x\") or panic(\"bad\");\n" +
	"    var o4 = os.Open(\"x\") or panic();\n" +
	"    var orAny = a or return;\n" +
	"    var u = -a + !b + ^c + ~d + &e + *p;\n" +
	"    var up = +a;\n" +
	"    var sel = a.b.c;\n" +
	"    var arr = u8[16] {};\n" +
	"    var gen = Stack[i32] {};\n" +
	"    var paren = (a + b);\n" +
	"    var r = 1 < 2 == true;\n" +
	"    var ex = append(xs, 1...);\n" +
	"    var dur = 10ms;\n" +
	"    var raw = `raw`;\n" +
	"    var bt = true;\n" +
	"    var mm = match (x) {\n" +
	"    case 0 => 1;;\n" +
	"    else => 2;\n" +
	"    };\n" +
	"    return;\n" +
	"};\n"

func TestExpressions(t *testing.T) {
	f := mustParse(t, expressionsSrc)
	checkSpans(t, f)

	p := singleValue(t, findVar(t, f, "p"))
	root, ok := p.(*ast.BinaryExpr)
	if !ok || root.Op != token.Minus {
		t.Fatalf("p = %#v", p)
	}
	plus, ok := root.X.(*ast.BinaryExpr)
	if !ok || plus.Op != token.Plus {
		t.Fatalf("p.X = %#v", root.X)
	}
	star, ok := plus.Y.(*ast.BinaryExpr)
	if !ok || star.Op != token.Star {
		t.Fatalf("p.X.Y = %#v", plus.Y)
	}
	if lit, ok := star.Y.(*ast.BasicLit); !ok || lit.Value != "3" {
		t.Errorf("3 literal = %#v", star.Y)
	}

	q := singleValue(t, findVar(t, f, "q")).(*ast.BinaryExpr)
	if q.Op != token.OrOr {
		t.Fatalf("q op = %v", q.Op)
	}
	and := q.Y.(*ast.BinaryExpr)
	if and.Op != token.AndAnd {
		t.Fatalf("q.Y op = %v", and.Op)
	}
	eq := and.Y.(*ast.BinaryExpr)
	if eq.Op != token.Eq {
		t.Fatalf("q.Y.Y op = %v", eq.Op)
	}
	add := eq.Y.(*ast.BinaryExpr)
	if add.Op != token.Plus {
		t.Fatalf("q.Y.Y.Y op = %v", add.Op)
	}
	if mul := add.Y.(*ast.BinaryExpr); mul.Op != token.Star {
		t.Fatalf("q.Y.Y.Y.Y op = %v", mul.Op)
	}

	if call := singleValue(t, findVar(t, f, "c1")).(*ast.CallExpr); len(call.Args) != 2 {
		t.Errorf("c1 args = %d", len(call.Args))
	}
	call2 := singleValue(t, findVar(t, f, "c2")).(*ast.CallExpr)
	sel, ok := call2.Fun.(*ast.SelectorExpr)
	if !ok || sel.Name != "M" {
		t.Errorf("c2 fun = %#v", call2.Fun)
	}
	if call3 := singleValue(t, findVar(t, f, "c3")).(*ast.CallExpr); len(call3.TypeArgs) != 0 {
		t.Errorf("c3 type args = %+v", call3.TypeArgs)
	}

	generic := singleValue(t, findVar(t, f, "c4")).(*ast.CallExpr)
	if len(generic.TypeArgs) != 2 {
		t.Fatalf("c4 type args = %d", len(generic.TypeArgs))
	}
	if nt, ok := generic.TypeArgs[0].(*ast.NamedType); !ok || nt.Name != "i32" {
		t.Errorf("c4 type arg 0 = %#v", generic.TypeArgs[0])
	}

	if _, ok := singleValue(t, findVar(t, f, "c5")).(*ast.IndexExpr); !ok {
		t.Errorf("c5 should be IndexExpr")
	}
	call6 := singleValue(t, findVar(t, f, "c6")).(*ast.CallExpr)
	if _, ok := call6.Fun.(*ast.IndexExpr); !ok {
		t.Errorf("c6 should call an index expression, got %#v", call6.Fun)
	}

	for name, want := range map[string][2]bool{
		"c7": {false, false},
		"c8": {true, false},
		"c9": {false, true},
	} {
		s := singleValue(t, findVar(t, f, name)).(*ast.SliceExpr)
		if (s.Low == nil) != want[0] || (s.High == nil) != want[1] {
			t.Errorf("%s low/high nil = %v/%v", name, s.Low == nil, s.High == nil)
		}
	}

	s1 := singleValue(t, findVar(t, f, "s1")).(*ast.CompositeLit)
	if len(s1.Entries) != 2 || s1.Entries[0].Key != "Name" {
		t.Fatalf("s1 entries = %+v", s1.Entries)
	}
	if lit, ok := s1.Entries[0].Value.(*ast.BasicLit); !ok || lit.Value != `"Alice"` || lit.Kind != token.String {
		t.Errorf("s1 value = %#v", s1.Entries[0].Value)
	}

	m1 := singleValue(t, findVar(t, f, "m1")).(*ast.CompositeLit)
	if _, ok := m1.Type.(*ast.MapType); !ok {
		t.Errorf("m1 type = %#v", m1.Type)
	}
	if len(m1.Entries) != 1 || m1.Entries[0].KeyExpr == nil {
		t.Errorf("m1 entries = %+v", m1.Entries)
	}
	m2 := singleValue(t, findVar(t, f, "m2")).(*ast.CompositeLit)
	if len(m2.Entries) != 1 || m2.Entries[0].KeyExpr == nil {
		t.Errorf("m2 identifier key should be KeyExpr: %+v", m2.Entries)
	}
	if id, ok := m2.Entries[0].KeyExpr.(*ast.Ident); !ok || id.Name != "key" {
		t.Errorf("m2 key = %#v", m2.Entries[0].KeyExpr)
	}

	sl := singleValue(t, findVar(t, f, "sl")).(*ast.CompositeLit)
	if _, ok := sl.Type.(*ast.SliceType); !ok || len(sl.Entries) != 3 {
		t.Errorf("sl = %+v", sl)
	}

	sh := singleValue(t, findVar(t, f, "sh")).(*ast.CompositeLit)
	if nt, ok := sh.Type.(*ast.NamedType); !ok || nt.Package != "Shape" || nt.Name != "Circle" {
		t.Errorf("sh type = %#v", sh.Type)
	}
	if len(sh.Entries) != 1 || sh.Entries[0].Key != "radius" {
		t.Errorf("sh entries = %+v", sh.Entries)
	}

	e1 := singleValue(t, findVar(t, f, "e1")).(*ast.CompositeLit)
	if nt, ok := e1.Type.(*ast.NamedType); !ok || nt.Package != "Shape" || nt.Name != "Empty" {
		t.Errorf("e1 type = %#v", e1.Type)
	}
	if len(e1.Entries) != 0 {
		t.Errorf("e1 entries = %+v", e1.Entries)
	}

	fl1 := singleValue(t, findVar(t, f, "fl1")).(*ast.FuncLit)
	if fl1.Arrow == nil || fl1.Body != nil || len(fl1.Results) != 0 {
		t.Errorf("fl1 = %+v", fl1)
	}
	if len(fl1.Params) != 2 || fl1.Params[0].Type != nil {
		t.Errorf("fl1 params = %+v", fl1.Params)
	}
	fl2 := singleValue(t, findVar(t, f, "fl2")).(*ast.FuncLit)
	if fl2.Arrow != nil || fl2.Body == nil || len(fl2.Results) == 0 {
		t.Errorf("fl2 = %+v", fl2)
	}
	if fl2.Params[0].Type == nil {
		t.Error("fl2 param type not parsed")
	}

	np := singleValue(t, findVar(t, f, "np")).(*ast.NilLit)
	if _, ok := np.Type.(*ast.PointerType); !ok {
		t.Errorf("np type = %#v", np.Type)
	}

	modes := map[ast.OrMode]int{}
	ast.Inspect(f, func(n ast.Node) bool {
		if or, ok := n.(*ast.OrExpr); ok {
			modes[or.Mode]++
		}
		return true
	})
	if modes[ast.OrReturn] != 2 || modes[ast.OrHandle] != 1 || modes[ast.OrPanic] != 2 {
		t.Errorf("or modes = %+v", modes)
	}
	if any := singleValue(t, findVar(t, f, "orAny")).(*ast.OrExpr); any.Mode != ast.OrReturn {
		t.Errorf("orAny = %+v", any)
	}
	if _, ok := singleValue(t, findVar(t, f, "orAny")).(*ast.OrExpr).X.(*ast.Ident); !ok {
		t.Errorf("orAny.X = %#v", singleValue(t, findVar(t, f, "orAny")).(*ast.OrExpr).X)
	}
	handle := singleValue(t, findVar(t, f, "o2")).(*ast.OrExpr)
	if handle.Name != "err" || handle.Block == nil {
		t.Errorf("o2 = %+v", handle)
	}
	o3 := singleValue(t, findVar(t, f, "o3")).(*ast.OrExpr)
	if o3.PanicArg == nil {
		t.Error("o3 panic arg nil")
	}
	o4 := singleValue(t, findVar(t, f, "o4")).(*ast.OrExpr)
	if o4.PanicArg != nil {
		t.Errorf("o4 panic arg = %#v", o4.PanicArg)
	}

	unaryOps := map[token.Kind]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if u, ok := n.(*ast.UnaryExpr); ok {
			unaryOps[u.Op] = true
		}
		return true
	})
	for _, op := range []token.Kind{token.Minus, token.Plus, token.Bang, token.Caret, token.Tilde, token.Amp, token.Star} {
		if !unaryOps[op] {
			t.Errorf("missing unary op %v", op)
		}
	}

	selABC := singleValue(t, findVar(t, f, "sel")).(*ast.SelectorExpr)
	if selABC.Name != "c" {
		t.Errorf("sel name = %q", selABC.Name)
	}
	if inner, ok := selABC.X.(*ast.SelectorExpr); !ok || inner.Name != "b" {
		t.Errorf("sel.X = %#v", selABC.X)
	}

	arr := singleValue(t, findVar(t, f, "arr")).(*ast.CompositeLit)
	arrType, ok := arr.Type.(*ast.ArrayType)
	if !ok {
		t.Fatalf("arr type = %#v", arr.Type)
	}
	if lit, ok := arrType.Len.(*ast.BasicLit); !ok || lit.Value != "16" {
		t.Errorf("arr len = %#v", arrType.Len)
	}

	gen := singleValue(t, findVar(t, f, "gen")).(*ast.CompositeLit)
	if gt, ok := gen.Type.(*ast.GenericType); !ok || len(gt.Args) != 1 {
		t.Errorf("gen type = %#v", gen.Type)
	}

	if _, ok := singleValue(t, findVar(t, f, "paren")).(*ast.ParenExpr); !ok {
		t.Error("paren should be ParenExpr")
	}

	r := singleValue(t, findVar(t, f, "r")).(*ast.BinaryExpr)
	if r.Op != token.Eq {
		t.Errorf("r op = %v", r.Op)
	}
	if x, ok := r.X.(*ast.BinaryExpr); !ok || x.Op != token.Lt {
		t.Errorf("r.X = %#v", r.X)
	}

	ex := singleValue(t, findVar(t, f, "ex")).(*ast.CallExpr)
	if !ex.Ellipsis {
		t.Error("ex ellipsis not set")
	}

	if d := singleValue(t, findVar(t, f, "dur")).(*ast.BasicLit); d.Kind != token.Duration || d.Value != "10ms" {
		t.Errorf("dur = %+v", d)
	}
	if raw := singleValue(t, findVar(t, f, "raw")).(*ast.BasicLit); raw.Kind != token.RawString || raw.Value != "`raw`" {
		t.Errorf("raw = %+v", raw)
	}
	if bt := singleValue(t, findVar(t, f, "bt")).(*ast.BasicLit); bt.Kind != token.True || bt.Value != "true" {
		t.Errorf("bt = %+v", bt)
	}

	mm := singleValue(t, findVar(t, f, "mm")).(*ast.MatchExpr)
	if !mm.IsExpr || len(mm.Cases) != 2 {
		t.Fatalf("mm = %+v", mm)
	}
	for i, c := range mm.Cases {
		if c.Expr == nil || c.Block != nil || len(c.Body) != 0 {
			t.Errorf("mm case %d = %+v", i, c)
		}
	}
}

const typesSrc = `type A = u8[];
type B = u8[16];
type C = u8[][];
type D = u8[16][];
type E = map[str, User];
type F = *Node;
type G = fn(A, B) C or error;
type H = (u8[])[];
type I = Stack[i32];
type J = pkg.Type;
type K = ed.Shape.Circle;
type L = u8[2 + 2];
type M = T[N];
type N = fn(A) B;
type P = interface {
    Close() void;
};
`

func TestTypes(t *testing.T) {
	f := mustParse(t, typesSrc)
	checkSpans(t, f)

	a := declType(t, f, "A").Target
	if st, ok := a.(*ast.SliceType); !ok {
		t.Fatalf("A = %#v", a)
	} else if nt, ok := st.Elem.(*ast.NamedType); !ok || nt.Name != "u8" {
		t.Errorf("A elem = %#v", st.Elem)
	}

	b := declType(t, f, "B").Target
	if at, ok := b.(*ast.ArrayType); !ok {
		t.Fatalf("B = %#v", b)
	} else if lit, ok := at.Len.(*ast.BasicLit); !ok || lit.Value != "16" {
		t.Errorf("B len = %#v", at.Len)
	}

	c := declType(t, f, "C").Target
	if st, ok := c.(*ast.SliceType); !ok {
		t.Fatalf("C = %#v", c)
	} else if _, ok := st.Elem.(*ast.SliceType); !ok {
		t.Errorf("C elem = %#v", st.Elem)
	}

	d := declType(t, f, "D").Target
	if st, ok := d.(*ast.SliceType); !ok {
		t.Fatalf("D = %#v", d)
	} else if _, ok := st.Elem.(*ast.ArrayType); !ok {
		t.Errorf("D elem = %#v", st.Elem)
	}

	e := declType(t, f, "E").Target
	if mt, ok := e.(*ast.MapType); !ok {
		t.Fatalf("E = %#v", e)
	} else if nt, ok := mt.Key.(*ast.NamedType); !ok || nt.Name != "str" {
		t.Errorf("E key = %#v", mt.Key)
	}

	fd := declType(t, f, "F").Target
	if pt, ok := fd.(*ast.PointerType); !ok {
		t.Fatalf("F = %#v", fd)
	} else if nt, ok := pt.Elem.(*ast.NamedType); !ok || nt.Name != "Node" {
		t.Errorf("F elem = %#v", pt.Elem)
	}

	g := declType(t, f, "G").Target
	if ft, ok := g.(*ast.FuncType); !ok {
		t.Fatalf("G = %#v", g)
	} else {
		if len(ft.Params) != 2 || !ft.Err {
			t.Errorf("G = %+v", ft)
		}
		if nt, ok := ft.Results[0].(*ast.NamedType); !ok || nt.Name != "C" {
			t.Errorf("G result = %#v", ft.Results[0])
		}
	}

	h := declType(t, f, "H").Target
	if st, ok := h.(*ast.SliceType); !ok {
		t.Fatalf("H = %#v", h)
	} else if _, ok := st.Elem.(*ast.SliceType); !ok {
		t.Errorf("H elem = %#v", st.Elem)
	}

	i := declType(t, f, "I").Target
	if gt, ok := i.(*ast.GenericType); !ok {
		t.Fatalf("I = %#v", i)
	} else if len(gt.Args) != 1 {
		t.Errorf("I args = %+v", gt.Args)
	}

	j := declType(t, f, "J").Target
	if nt, ok := j.(*ast.NamedType); !ok || nt.Package != "pkg" || nt.Name != "Type" {
		t.Errorf("J = %#v", j)
	}

	k := declType(t, f, "K").Target
	if nt, ok := k.(*ast.NamedType); !ok || nt.Package != "ed.Shape" || nt.Name != "Circle" {
		t.Errorf("K = %#v", k)
	}

	l := declType(t, f, "L").Target
	if at, ok := l.(*ast.ArrayType); !ok {
		t.Fatalf("L = %#v", l)
	} else if bin, ok := at.Len.(*ast.BinaryExpr); !ok || bin.Op != token.Plus {
		t.Errorf("L len = %#v", at.Len)
	}

	m := declType(t, f, "M").Target
	if gt, ok := m.(*ast.GenericType); !ok {
		t.Fatalf("M = %#v", m)
	} else if nt, ok := gt.Args[0].(*ast.NamedType); !ok || nt.Name != "N" {
		t.Errorf("M arg = %#v", gt.Args[0])
	}

	n := declType(t, f, "N").Target
	if ft, ok := n.(*ast.FuncType); !ok || ft.Err || len(ft.Results) == 0 {
		t.Errorf("N = %#v", n)
	}

	p := declType(t, f, "P")
	if p.Kind != ast.TypeInterface || len(p.IFace) != 1 || p.IFace[0].Name != "Close" {
		t.Errorf("P = %+v", p)
	}
}

const specExampleSrc = `use fmt;
use os;
use sort;

type User = struct {
    Name: str,
    age: u32,

    fn Greet() str = {
        return "hi, " + Name;
    };
};

type Shape = enum {
    Circle { radius: f64 },
    Rect { w: f64, h: f64 },
};

fn area(s: Shape) f64 = {
    return match (s) {
    case Circle { radius } => 3.14159 * radius * radius;
    case Rect { w, h } => w * h;
    };
};

fn read(path: str) u8[] or error = {
    var f = os.Open(path) or return;
    defer f.Close();
    var buf = u8[4096] {};
    var n = f.Read(buf[:]) or return;
    return buf[:n];
};

fn main() void or error = {
    var users = User[] {
        User { Name = "Alice", age = 30 },
        User { Name = "Bob", age = 25 },
    };

    sort.Slice(users, fn(i, j) => users[i].age < users[j].age);

    for (var u in users) {
        fmt.Println(u.Greet()) or return;
    };

    var data = read("users.txt") or err {
        fmt.Println("read failed:", err) or return;
        return err;
    };

    fmt.Println(len(data)) or return;
};
`

func TestSpecExample(t *testing.T) {
	f := mustParse(t, specExampleSrc)
	checkSpans(t, f)

	area := declFunc(t, f, "area")
	if area == nil || area.Body == nil || len(area.Body.Stmts) != 1 {
		t.Fatalf("area = %+v", area)
	}
	ret, ok := area.Body.Stmts[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		t.Fatalf("area return = %#v", area.Body.Stmts[0])
	}
	m, ok := ret.Results[0].(*ast.MatchExpr)
	if !ok || !m.IsExpr || len(m.Cases) != 2 {
		t.Errorf("area match = %#v", ret.Results[0])
	}
	if ok {
		for i, c := range m.Cases {
			if c.Expr == nil {
				t.Errorf("area match case %d has no expression", i)
			}
		}
	}

	main := declFunc(t, f, "main")
	var ors int
	ast.Inspect(main.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.OrExpr); ok {
			ors++
		}
		return true
	})
	if ors != 4 {
		t.Errorf("main or count = %d, want 4", ors)
	}
}

func TestComments(t *testing.T) {
	src := "// head\nuse fmt; // mid\nfn f() void = {\n    // body\n};\n"
	f := mustParse(t, src)
	checkSpans(t, f)

	if len(f.Comments) != 3 {
		t.Fatalf("comments = %d, want 3", len(f.Comments))
	}
	want := []string{"// head", "// mid", "// body"}
	for i, w := range want {
		c := f.Comments[i]
		if c.Text != w || c.Block {
			t.Errorf("comment %d = %+v", i, c)
		}
		if i > 0 && c.Pos().Offset < f.Comments[i-1].Pos().Offset {
			t.Errorf("comment %d out of order", i)
		}
	}
}

func TestBlockCommentRejected(t *testing.T) {
	src := "// head\nuse fmt; /* mid */\nfn f() void = {\n};\n"
	f, errs := Parse("block.str", []byte(src))
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	if errs[0].Msg != "block comments are not supported; use //" {
		t.Fatalf("error = %q", errs[0].Msg)
	}
	if len(f.Comments) != 1 || f.Comments[0].Text != "// head" {
		t.Fatalf("comments = %+v", f.Comments)
	}
}

func TestMalformedDoesNotPanic(t *testing.T) {
	cases := []string{
		"fn",
		"fn f( {",
		"type = ;",
		"var = ;",
		"fn f() void = {",
		"fn f() void = { var x = ; };",
		"if (",
		"match (x) { case => ;",
		"fn f() void = { x = [; };",
		"@@@ ???",
		"type X = struct { a: int fn };",
		"test \"x\" {",
		"(((((((((",
		"fn f() void = { defer 1; };",
		"type X = enum { A { ; } };",
		"fn f() void = { return 1 2; };",
		"type M = map[str; };",
		"fn f() void = { or return; };",
		"fn f() void = { match (a) { case Circle { radius } => ; }; };",
		"type X = struct { a: };",
		"fn f() void = { case 1 => 2; };",
	}
	for _, src := range cases {
		f, errs := Parse("bad.str", []byte(src))
		if f == nil {
			t.Fatalf("nil file for %q", src)
		}
		if len(errs) == 0 {
			t.Errorf("no errors for %q", src)
		}
		if len(errs) > maxErrors {
			t.Errorf("%d errors for %q, cap is %d", len(errs), src, maxErrors)
		}
		checkSpans(t, f)
	}

	f, errs := Parse("empty.str", nil)
	if f == nil || len(errs) != 0 || len(f.Decls) != 0 || len(f.Uses) != 0 {
		t.Errorf("empty file = %+v, errs = %+v", f, errs)
	}
	if f.Pos().Line != 1 || f.EndPos().Offset != 0 {
		t.Errorf("empty file span = %+v", f.Span)
	}
}

func TestRecoveryKeepsLaterDecls(t *testing.T) {
	src := `var = ;
type Good = struct {
    ok: int,

    fn Ping() void = {
        return;
    };
};
fn fine() void = {
    return;
};
`
	f, errs := Parse("recover.str", []byte(src))
	if len(errs) == 0 {
		t.Fatal("expected errors")
	}
	checkSpans(t, f)

	good := declType(t, f, "Good")
	if len(good.Fields) != 1 || len(good.Methods) != 1 {
		t.Errorf("Good = %+v", good)
	}
	fine := declFunc(t, f, "fine")
	if fine.Body == nil || len(fine.Body.Stmts) != 1 {
		t.Errorf("fine = %+v", fine)
	}
}

func TestErrorCap(t *testing.T) {
	src := strings.Repeat("var = ;\n", maxErrors+50)
	f, errs := Parse("cap.str", []byte(src))
	if f == nil {
		t.Fatal("nil file")
	}
	if len(errs) != maxErrors {
		t.Fatalf("errors = %d, want %d", len(errs), maxErrors)
	}
	checkSpans(t, f)
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(realisticSrc))
	f.Add([]byte(statementsSrc))
	f.Add([]byte(expressionsSrc))
	f.Add([]byte(typesSrc))
	for _, src := range []string{
		"fn f() void = {",
		"match (x) { case => ;",
		"type X = struct { a: int fn };",
		"@@@",
	} {
		f.Add([]byte(src))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		file, errs := Parse("fuzz.str", src)
		if file == nil {
			t.Fatal("nil file")
		}
		if len(errs) > maxErrors {
			t.Fatalf("%d errors, cap is %d", len(errs), maxErrors)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			start, end := n.Pos(), n.EndPos()
			if start.Line <= 0 || end.Line <= 0 {
				t.Fatalf("%T: invalid line %+v..%+v", n, start, end)
			}
			if end.Offset < start.Offset || end.Offset > len(src) {
				t.Fatalf("%T: bad span %d..%d (src %d)", n, start.Offset, end.Offset, len(src))
			}
			return true
		})
	})
}
