package parser

import (
	"testing"

	"github.com/bongdo/strizh/internal/ast"
)

func TestResultList(t *testing.T) {
	f := mustParse(t, `
fn pair() i32, i32 or error = {
    return 1, 2;
};

type Handler = fn(i32) (str, bool);

fn main() void = {};
`)
	fn := declFunc(t, f, "pair")
	if len(fn.Results) != 2 || !fn.Err {
		t.Fatalf("pair = %+v", fn)
	}
	h := declType(t, f, "Handler").Target.(*ast.FuncType)
	if len(h.Results) != 2 || h.Err {
		t.Fatalf("Handler = %+v", h)
	}
}

func TestTypePatternAndElse(t *testing.T) {
	f := mustParse(t, `
fn main() void = {
    match (msg) {
    case tea.KeyPressMsg as k => {};
    case tea.WindowSizeMsg => {};
    else => {};
    };
};
`)
	m, ok := declFunc(t, f, "main").Body.Stmts[0].(*ast.ExprStmt).X.(*ast.MatchExpr)
	if !ok {
		t.Fatalf("match not parsed")
	}
	if len(m.Cases) != 3 {
		t.Fatalf("cases = %d", len(m.Cases))
	}
	if m.Cases[0].Pat.Bind != "k" || m.Cases[0].Pat.Name != "KeyPressMsg" || m.Cases[0].Pat.Qualifier != "tea" {
		t.Fatalf("first case = %+v", m.Cases[0].Pat)
	}
	if !m.Cases[2].Default {
		t.Fatalf("else not marked default")
	}
}

func TestBareNil(t *testing.T) {
	f := mustParse(t, `
fn main() void = {
    var x = nil;
    return nil;
};
`)
	fn := declFunc(t, f, "main")
	decl := fn.Body.Stmts[0].(*ast.DeclStmt).Decl
	if n, ok := decl.Values[0].(*ast.NilLit); !ok || n.Type != nil {
		t.Fatalf("nil = %#v", decl.Values[0])
	}
	ret := fn.Body.Stmts[1].(*ast.ReturnStmt)
	if n, ok := ret.Results[0].(*ast.NilLit); !ok || n.Type != nil {
		t.Fatalf("return nil = %#v", ret.Results[0])
	}
}
