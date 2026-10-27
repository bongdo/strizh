package sema

import "testing"

func hasError(p *Program, want string) bool {
	for _, e := range p.Errors {
		if contains(e.Msg, want) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}

func TestMatchAnyRequiresElse(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `
fn f(v: any) str = {
    return match (v) {
    case i32 as n => "i";
    };
};
fn main() void = {};
`})
	if !hasError(p, "requires else") {
		t.Fatalf("errors = %v", p.Errors)
	}
}

func TestTypePatternOnConcrete(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `
fn f(n: i32) str = {
    return match (n) {
    case i32 as x => "i";
    else => "e";
    };
};
fn main() void = {};
`})
	if !hasError(p, "type patterns are only allowed") {
		t.Fatalf("errors = %v", p.Errors)
	}
}

func TestNilNotNilable(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `
fn f() i32 = {
    return nil;
};
fn main() void = {};
`})
	if !hasError(p, "nil is not valid") {
		t.Fatalf("errors = %v", p.Errors)
	}
}

func TestMultiResultArity(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `
fn f() i32, i32 = {
    return 1;
};
fn main() void = {};
`})
	if !hasError(p, "wrong number of return values") {
		t.Fatalf("errors = %v", p.Errors)
	}
}

func TestMultiResultInference(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `
fn f() i32, bool = {
    return 1, true;
};
fn main() void = {
    var a, b = f();
    a = a;
    b = b;
};
`})
	noErrors(t, p)
}

func TestTypePatternBinding(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `
fn f(v: any) str = {
    return match (v) {
    case i32 as n => "i";
    else => "e";
    };
};
fn main() void = {};
`})
	noErrors(t, p)
}

func TestBareNilExpected(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `
fn f() error = {
    return nil;
};
fn main() void = {
    var e = f();
    if (e == nil) {};
};
`})
	noErrors(t, p)
}
