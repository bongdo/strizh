package sema

import "testing"

func parGapsOK(t *testing.T, src string) {
	t.Helper()
	p := checkSrc(t, map[string]string{".": src})
	noErrors(t, p)
}

func parGapsError(t *testing.T, src string) {
	t.Helper()
	p := checkSrc(t, map[string]string{".": src})
	if len(p.Errors) == 0 {
		t.Fatal("expected a par write error")
	}
}

func TestParForMapWriteRejected(t *testing.T) {
	parGapsError(t, `fn main() void = {
    var m = map[i32, i32] {};
    par for (var i = 0; i < 4; i += 1) {
        m[i] = i;
    };
};
`)
}

func TestParForMapFieldWriteRejected(t *testing.T) {
	parGapsError(t, `type Cell = struct { v: i32 };

fn main() void = {
    var m = map[i32, Cell] {};
    par for (var i = 0; i < 4; i += 1) {
        m[i].v = i;
    };
};
`)
}

func TestParForNestedMapWriteRejected(t *testing.T) {
	parGapsError(t, `type Group = struct { m: map[i32, i32] };

fn main() void = {
    var g = Group { m = map[i32, i32] {} };
    par for (var i = 0; i < 4; i += 1) {
        g.m[i] = i;
    };
};
`)
}

func TestParForSliceFieldWriteAllowed(t *testing.T) {
	parGapsOK(t, `type Cell = struct { v: i32 };

fn main() void = {
    var cs = Cell[] { Cell { v = 0 }, Cell { v = 0 } };
    par for (var i = 0; i < 2; i += 1) {
        cs[i].v = i;
    };
};
`)
}

func TestParPlainMapWriteRejected(t *testing.T) {
	parGapsError(t, `fn main() void = {
    var m = map[i32, i32] {};
    par {
        m[0] = 1;
    };
};
`)
}

func TestParForAliasFieldWriteRejected(t *testing.T) {
	parGapsError(t, `type Box = struct { x: i32 };

fn main() void = {
    var b = Box { x = 0 };
    var p = &b;
    par for (var i = 0; i < 2; i += 1) {
        var q = p;
        q.x = i;
    };
};
`)
}

func TestParForAliasAssignFieldWriteRejected(t *testing.T) {
	parGapsError(t, `type Box = struct { x: i32 };

fn main() void = {
    var b = Box { x = 0 };
    var p = &b;
    par for (var i = 0; i < 2; i += 1) {
        var q = p;
        q = p;
        q.x = i;
    };
};
`)
}

func TestParForAddressAliasFieldWriteRejected(t *testing.T) {
	parGapsError(t, `type Box = struct { x: i32 };

fn main() void = {
    var b = Box { x = 0 };
    par for (var i = 0; i < 2; i += 1) {
        var q = &b;
        q.x = i;
    };
};
`)
}

func TestParForAliasChainFieldWriteRejected(t *testing.T) {
	parGapsError(t, `type Box = struct { x: i32 };

fn main() void = {
    var b = Box { x = 0 };
    var p = &b;
    par for (var i = 0; i < 2; i += 1) {
        var q = p;
        var r = q;
        r.x = i;
    };
};
`)
}

func TestParForLocalAliasFieldWriteAllowed(t *testing.T) {
	parGapsOK(t, `type Box = struct { x: i32 };

fn main() void = {
    par for (var i = 0; i < 2; i += 1) {
        var b = Box { x = 0 };
        var q = &b;
        q.x = i;
    };
};
`)
}

func TestParForAliasMethodRejected(t *testing.T) {
	parGapsError(t, `type Counter = struct {
    n: i32,

    fn Add(v: i32) void = {
        n = n + v;
    };
};

fn main() void = {
    var c = Counter { n = 0 };
    var p = &c;
    par for (var i = 0; i < 2; i += 1) {
        var q = p;
        q.Add(i);
    };
};
`)
}

func TestParForCapturedMethodRejected(t *testing.T) {
	parGapsError(t, `type Counter = struct {
    n: i32,

    fn Add(v: i32) void = {
        n = n + v;
    };
};

fn main() void = {
    var c = Counter { n = 0 };
    var p = &c;
    par for (var i = 0; i < 2; i += 1) {
        p.Add(i);
    };
};
`)
}

func TestParPureMethodAllowed(t *testing.T) {
	parGapsOK(t, `type Counter = struct {
    n: i32,

    fn Get() i32 = {
        return n;
    };
};

fn main() void = {
    var c = Counter { n = 1 };
    var p = &c;
    var a = 0;
    var b = 0;
    par {
        a = p.Get();
        b = p.Get();
    };
};
`)
}

func TestParForLocalMethodAllowed(t *testing.T) {
	parGapsOK(t, `type Counter = struct {
    n: i32,

    fn Add(v: i32) void = {
        n = n + v;
    };
};

fn main() void = {
    par for (var i = 0; i < 2; i += 1) {
        var c = Counter { n = 0 };
        c.Add(i);
    };
};
`)
}

func TestParForSliceSlotMethodAllowed(t *testing.T) {
	parGapsOK(t, `type Cell = struct {
    v: i32,

    fn Set(x: i32) void = {
        v = x;
    };
};

fn main() void = {
    var cs = Cell[] { Cell { v = 0 }, Cell { v = 0 } };
    par for (var i = 0; i < 2; i += 1) {
        cs[i].Set(i);
    };
};
`)
}

func TestParMethodFieldWriteRejected(t *testing.T) {
	parGapsError(t, `type Counter = struct {
    n: i32,

    fn Bump() void = {
        par {
            n = 1;
        };
    };
};
`)
}

func TestParReadInOtherStatementRejected(t *testing.T) {
	parGapsError(t, `fn load(x: i32) i32 = {
    return x;
};

fn main() void = {
    var a = 0;
    var b = 0;
    par {
        a = load(1);
        b = a;
    };
};
`)
}

func TestParIndependentWritesAllowed(t *testing.T) {
	parGapsOK(t, `fn load(x: i32) i32 = {
    return x;
};

fn main() void = {
    var a = 0;
    var b = 0;
    par {
        a = load(1);
        b = load(2);
    };
};
`)
}

func TestParReadOwnWriteAllowed(t *testing.T) {
	parGapsOK(t, `fn main() void = {
    var a = 0;
    par {
        a = a + 1;
    };
};
`)
}

func TestParReadViaAliasRejected(t *testing.T) {
	parGapsError(t, `fn main() void = {
    var xs = i32[] { 0 };
    var b = 0;
    par {
        var q = xs;
        q[0] = 1;
        b = xs[0];
    };
};
`)
}
