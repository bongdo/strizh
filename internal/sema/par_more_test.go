package sema

import "testing"

func TestParForIterVarWriteAllowed(t *testing.T) {
	parGapsOK(t, `
fn main() void = {
    var xs = i32[] {1, 2, 3};
    par for (var i, x in xs) {
        i = i + 1;
        x = x;
    };
    par for (var j = 0; j < 3; j += 1) {
        j = j + 1;
    };
};
`)
}

func TestParForAssignInitWriteRejected(t *testing.T) {
	parGapsError(t, `
fn main() void = {
    var i = 0;
    par for (i = 0; i < 3; i += 1) {
        i = i + 1;
    };
};
`)
}

func TestParForNonIdentInitRejected(t *testing.T) {
	parGapsError(t, `
fn main() void = {
    var a = i32[1] {};
    par for (a[0] = 0; a[0] < 3; a[0] += 1) {
    };
};
`)
}

func TestParMatchWriteRejected(t *testing.T) {
	parGapsError(t, `
fn main() void = {
    var a = 0;
    var b = 0;
    par {
        match (1) {
        case 1 => { a = 1; };
        else => {};
        };
        b = a + 1;
    };
};
`)
}

func TestParOrBlockWriteRejected(t *testing.T) {
	parGapsError(t, `
use errors;

fn fail() void or error = {
    return errors.New("x");
};

fn main() void or error = {
    var a = 0;
    var b = 0;
    par {
        fail() or err {
            a = 1;
        };
        b = a + 1;
    };
};
`)
}

func TestParGlobalWriteRejected(t *testing.T) {
	parGapsError(t, `
var counter = 0;

fn main() void = {
    par {
        counter = 1;
    };
};
`)
}

func TestParStructMapWriteRejected(t *testing.T) {
	parGapsError(t, `
type Group = struct {
    m: map[i32, i32],
};

fn main() void = {
    var g = Group { m = map[i32, i32] {} };
    par {
        g.m[0] = 1;
    };
};
`)
}

func TestParFuncLitState(t *testing.T) {
	parGapsOK(t, `
fn main() void = {
    par {
        {
            var f = fn(x: i32) i32 {
                return x + 1;
            };
            f = f;
        };
    };
};
`)
	parGapsError(t, `
fn main() void = {
    for (var i = 0; i < 1; i += 1) {
        var f = fn() void {
            break;
        };
        f = f;
    };
};
`)
}

func TestParMethodMatchWriteRejected(t *testing.T) {
	parGapsError(t, `
type C = struct {
    n: i32,

    fn Get() i32 = {
        return match (1) {
        case 1 => {
            n = 1;
            return n;
        };
        else => 0;
        };
    };
};

fn main() void = {
    var c = C { n = 0 };
    var a = 0;
    var b = 0;
    par {
        a = c.Get();
        b = c.n;
    };
};
`)
}
