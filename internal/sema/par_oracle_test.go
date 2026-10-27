package sema

import "testing"

func parCheck(t *testing.T, src string) *Program {
	t.Helper()
	return checkSrc(t, map[string]string{".": src})
}

func parWant(t *testing.T, src, want string) {
	t.Helper()
	p := parCheck(t, src)
	if !hasError(p, want) {
		t.Fatalf("want error containing %q, got %v", want, p.Errors)
	}
}

func parOK(t *testing.T, src string) {
	t.Helper()
	p := parCheck(t, src)
	if len(p.Errors) == 0 {
		return
	}
	noErrors(t, p)
}

func TestParMapWriteRejected(t *testing.T) {
	cases := map[string]string{
		"plain par": `fn main() void = {
    var m = map[i32, i32] {};
    par {
        m[0] = 1;
    };
};
`,
		"range map": `fn main() void = {
    var m = map[i32, i32] { 1 = 1 };
    par for (var k, v in m) {
        m[k] = v;
    };
};
`,
		"c-style": `fn main() void = {
    var m = map[i32, i32] {};
    par for (var i = 0; i < 2; i += 1) {
        m[i] = i;
    };
};
`,
		"nested through map": `fn main() void = {
    var mm = map[i32, i32[]] {};
    par for (var i = 0; i < 2; i += 1) {
        mm[i][0] = i;
    };
};
`,
		"disjoint key map": `fn main() void = {
    var xs = i32[] {1, 2};
    var out = map[i32, i32] {};
    par for (var i, x in xs) {
        out[i] = x;
    };
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parWant(t, src, "writing to map")
		})
	}
}

func TestParPointerWriteRejected(t *testing.T) {
	cases := map[string]string{
		"field": `type C = struct {
    n: i32,
};

fn main() void = {
    var c = C { n = 0 };
    var p = &c;
    par {
        p.n = 1;
    };
};
`,
		"deref": `fn main() void = {
    var n = 0;
    var p = &n;
    par {
        *p = 2;
    };
};
`,
		"slot": `fn main() void = {
    var xs = i32[2] {};
    var p = &xs;
    par {
        p[0] = 3;
    };
};
`,
		"param": `type C = struct {
    n: i32,
};

fn bump(p: *C) void = {
    par {
        p.n = 1;
    };
};

fn main() void = {
    var c = C { n = 0 };
    bump(&c);
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parWant(t, src, "captured from outside par")
		})
	}
}

func TestParAliasWriteRejected(t *testing.T) {
	cases := map[string]string{
		"alias outside": `type C = struct {
    n: i32,
};

fn main() void = {
    var c = C { n = 0 };
    var p = &c;
    var q = p;
    par {
        q.n = 1;
    };
};
`,
		"alias inside": `type C = struct {
    n: i32,
};

fn main() void = {
    var c = C { n = 0 };
    var p = &c;
    par {
        {
            var q = p;
            q.n = 1;
        };
    };
};
`,
		"chained alias inside": `type C = struct {
    n: i32,
};

fn main() void = {
    var c = C { n = 0 };
    var p = &c;
    par {
        {
            var q = p;
            var r = q;
            r.n = 1;
        };
    };
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parWant(t, src, "captured from outside par")
		})
	}
}

func TestParMethodWriteRejected(t *testing.T) {
	cases := map[string]string{
		"pointer receiver": `type C = struct {
    n: i32,

    fn Bump() void = {
        n += 1;
    };
};

fn main() void = {
    var c = C { n = 0 };
    var p = &c;
    par {
        p.Bump();
    };
};
`,
		"self": `type C = struct {
    n: i32,

    fn Bump() void = {
        n += 1;
    };

    fn Run() void = {
        par {
            self.Bump();
        };
    };
};

fn main() void = {
    var c = C { n = 0 };
    c.Run();
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			want := "captured from outside par"
			if name == "self" {
				want = "writing to self inside par"
			}
			parWant(t, src, want)
		})
	}
}

func TestParSharedSliceRejected(t *testing.T) {
	cases := map[string]string{
		"two slots": `fn main() void = {
    var out = i32[2] {};
    par {
        out[0] = 1;
        out[1] = 2;
    };
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parWant(t, src, "captured from outside par")
		})
	}
}

func TestParReadWriteRejected(t *testing.T) {
	cases := map[string]string{
		"read after write": `fn id(n: i32) i32 = {
    return n;
};

fn main() void = {
    var a = 0;
    var b = 0;
    par {
        a = id(1);
        b = a + 1;
    };
};
`,
		"read as argument": `fn id(n: i32) i32 = {
    return n;
};

fn main() void = {
    var a = 0;
    var b = 0;
    par {
        a = id(1);
        b = id(a);
    };
};
`,
		"read in condition": `fn id(n: i32) i32 = {
    return n;
};

fn main() void = {
    var a = 0;
    var b = 0;
    par {
        a = id(1);
        if (a > 0) {
            b = 2;
        };
    };
};
`,
		"read in nested loop": `fn id(n: i32) i32 = {
    return n;
};

fn main() void = {
    var a = 0;
    var b = 0;
    par {
        a = id(1);
        for (var k = 0; k < a; k += 1) {
            b += k;
        };
    };
};
`,
		"slot written and read": `fn main() void = {
    var out = i32[2] {};
    var n = 0;
    par {
        out[0] = 1;
        n = out[1];
    };
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parWant(t, src, "read by another par statement")
		})
	}
}

func TestParWriteSetsAllowed(t *testing.T) {
	cases := map[string]string{
		"distinct writers": `fn id(n: i32) i32 = {
    return n;
};

fn main() void = {
    var a = 0;
    var b = 0;
    par {
        a = id(1);
        b = id(2);
    };
};
`,
		"readers only": `fn main() void = {
    var a = 1;
    var b = 0;
    var c = 0;
    par {
        b = a + 1;
        c = a + 2;
    };
};
`,
		"single writer": `fn id(n: i32) i32 = {
    return n;
};

fn main() void = {
    var a = 0;
    par {
        a = id(1);
    };
};
`,
		"single statement reads and writes": `fn main() void = {
    var a = 0;
    par {
        a += 1;
    };
};
`,
		"single slice slot": `fn main() void = {
    var out = i32[1] {};
    par {
        out[0] = 1;
    };
};
`,
		"value struct method": `type C = struct {
    n: i32,

    fn Bump() void = {
        n += 1;
    };
};

fn main() void = {
    var c = C { n = 0 };
    par {
        c.Bump();
    };
};
`,
		"disjoint slots": `fn id(n: i32) i32 = {
    return n;
};

fn main() void = {
    var xs = i32[] {1, 2, 3};
    var out = i32[3] {};
    par for (var i, x in xs) {
        out[i] = id(x);
    };
};
`,
		"map reads": `fn main() void = {
    var m = map[i32, i32] { 1 = 2, 2 = 3 };
    var out = i32[4] {};
    par for (var k, v in m) {
        out[k] = v;
    };
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parOK(t, src)
		})
	}
}

func TestParAllowsWithin(t *testing.T) {
	cases := map[string]string{
		"par": `fn main() void = {
    var a = 0;
    par {
        within 5s {
            a = 1;
        };
    };
};
`,
		"par for": `fn main() void = {
    var out = i32[2] {};
    par for (var i = 0; i < 2; i += 1) {
        within 5s {
            out[i] = i;
        };
    };
};
`,
		"nested within": `fn main() void = {
    var a = 0;
    par {
        within 5s {
            within 1s {
                a = 1;
            };
        };
    };
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parOK(t, src)
		})
	}
}

func TestParAllowsDefer(t *testing.T) {
	cases := map[string]string{
		"par": `fn note() void = {
};

fn main() void = {
    par {
        defer note();
        note();
    };
};
`,
		"par for": `fn note() void = {
};

fn main() void = {
    par for (var i = 0; i < 2; i += 1) {
        defer note();
        note();
    };
};
`,
		"with within and or return": `fn note() void = {
};

fn load() i32 or error = {
    return 1;
};

fn main() void or error = {
    var a = 0;
    par {
        defer note();
        within 5s {
            defer note();
            a = load() or return;
        };
    };
    a += 1;
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parOK(t, src)
		})
	}
}

func TestParAllowsOrPanic(t *testing.T) {
	cases := map[string]string{
		"message": `fn load() i32 or error = {
    return 1;
};

fn main() void = {
    var a = 0;
    par {
        a = load() or panic("boom");
    };
};
`,
		"error value": `fn load() i32 or error = {
    return 1;
};

fn main() void = {
    var a = 0;
    par {
        a = load() or panic();
    };
};
`,
		"par for": `fn load() i32 or error = {
    return 1;
};

fn main() void = {
    var out = i32[2] {};
    par for (var i = 0; i < 2; i += 1) {
        out[i] = load() or panic("boom");
    };
};
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			parOK(t, src)
		})
	}
}

func TestParForCstyleIterVar(t *testing.T) {
	parOK(t, `fn main() void = {
    var out = i32[4] {};
    var i = 0;
    par for (i = 0; i < 4; i += 1) {
        out[i] = i * 2;
    };
};
`)
	parWant(t, `fn main() void = {
    var i = 0;
    par for (i = 0; i < 4; i += 1) {
        i = i + 1;
    };
};
`, "captured from outside par")
}

func TestParBoundaryStillRejected(t *testing.T) {
	parWant(t, `fn main() void = {
    par {
        return;
    };
};
`, "cannot leave a par block")
	parWant(t, `fn main() void = {
    var out = i32[2] {};
    par for (var i = 0; i < 2; i += 1) {
        out[i] = i;
        break;
    };
};
`, "break cannot leave a par block")
	parWant(t, `fn main() void = {
    var out = i32[2] {};
    par for (var i = 0; i < 2; i += 1) {
        out[i] = i;
        continue;
    };
};
`, "continue cannot leave a par block")
}
