package gen

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected an exit error, got %v", err)
	}
	return ee.ExitCode()
}

func TestParPanicBecomesError(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use errors;
use "os/exec";

fn fail() i32 or error = {
    return 0, errors.New("sibling");
};

fn work() void or error = {
    var x = i32(0);
    par {
        panic("kaboom");
        {
            var cmd = exec.CommandContext("sleep", "2");
            var err = cmd.Run();
            if (err == nil[error]) {
                fmt.Println("sibling-finished") or return;
            };
        };
        x = fail() or return;
    };
    fmt.Println(x) or return;
};

fn main() void or error = {
    work() or return;
    fmt.Println("done") or return;
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err == nil {
		t.Fatalf("expected failure, stdout:\n%s", stdout)
	}
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "kaboom") {
		t.Fatalf("stderr does not contain the panic message: %s", stderr)
	}
	if !strings.Contains(stderr, "main.str:12") {
		t.Fatalf("stderr does not point at the panic site main.str:12:\n%s", stderr)
	}
	if strings.Contains(stderr, "sibling") {
		t.Fatalf("an error from a higher-index sibling won: %s", stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}

func TestParPanicVoidMain(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

fn boom() void = {
    panic("kaboom");
};

fn main() void = {
    par {
        boom();
    };
    fmt.Println("after");
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err == nil {
		t.Fatalf("expected a panic, stdout:\n%s", stdout)
	}
	if code := exitCode(t, err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (a panic)\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "kaboom") {
		t.Fatalf("stderr does not contain the panic message: %s", stderr)
	}
	if strings.Contains(stdout, "after") {
		t.Fatalf("statement after par ran:\n%s", stdout)
	}
}

func TestParPanicCancelsPlainPar(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use "os/exec";

fn main() void or error = {
    par {
        panic("kaboom");
        {
            var cmd = exec.CommandContext("sleep", "2");
            var err = cmd.Run();
            if (err == nil[error]) {
                fmt.Println("sibling-finished") or return;
            };
        };
    };
    fmt.Println("done") or return;
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err == nil {
		t.Fatalf("expected failure, stdout:\n%s", stdout)
	}
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "kaboom") {
		t.Fatalf("stderr does not contain the panic message: %s", stderr)
	}
	if stdout != "" {
		t.Fatalf("sibling was not cancelled, stdout:\n%s", stdout)
	}
}

func TestParWithinCancels(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use "os/exec";

fn main() void or error = {
    par {
        {
            within 200ms {
                var cmd = exec.CommandContext("sleep", "3");
                var err = cmd.Run();
                if (err == nil[error]) {
                    fmt.Println("finished") or return;
                };
                fmt.Println("cancelled") or return;
            };
        };
    };
    fmt.Println("done") or return;
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "cancelled\ndone\n")
}

func TestParWithinInside(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

fn work(n: i32) i32 = {
    return n * 10;
};

fn main() void or error = {
    var a = i32(0);
    var b = i32(0);
    par {
        within 5s {
            a = work(1);
        };
        within 5s {
            b = work(2);
        };
    };
    fmt.Println(a, b) or return;
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "10 20\n")
}

func TestParDeferInside(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;
use errors;

fn fail() void or error = {
    return errors.New("stop");
};

fn work() void or error = {
    par {
        {
            defer fmt.Println("cleanup");
            fail() or return;
            fmt.Println("unreachable") or return;
        };
    };
    fmt.Println("unreachable") or return;
};

fn main() void or error = {
    par {
        {
            defer fmt.Println("d1");
            defer fmt.Println("d2");
            fmt.Println("body") or return;
        };
    };
    var caught = 0;
    work() or err {
        caught = 1;
    };
    fmt.Println("caught", caught) or return;
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "body\nd2\nd1\ncleanup\ncaught 1\n")
}

func TestParOrPanic(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "message",
			src: `use errors;

fn fetch() i32 or error = {
    return 0, errors.New("fetch");
};

fn work() void or error = {
    var x = i32(0);
    par {
        x = fetch() or panic("boom");
    };
    x += 1;
};

fn main() void or error = {
    work() or return;
};
`,
			want: "boom",
		},
		{
			name: "error value",
			src: `use errors;

fn fetch() i32 or error = {
    return 0, errors.New("fetch");
};

fn work() void or error = {
    var x = i32(0);
    par {
        x = fetch() or panic();
    };
    x += 1;
};

fn main() void or error = {
    work() or return;
};
`,
			want: "fetch",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newProject(t, map[string]string{"main.str": c.src})
			stdout, stderr, err := p.run(t)
			if err == nil {
				t.Fatalf("expected failure, stdout:\n%s", stdout)
			}
			if code := exitCode(t, err); code != 1 {
				t.Fatalf("exit code = %d, want 1 (not a Go panic)\nstderr:\n%s", code, stderr)
			}
			if !strings.Contains(stderr, c.want) {
				t.Fatalf("stderr does not contain %q: %s", c.want, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
		})
	}
}

func TestParForCstyleCapture(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use fmt;

fn spin() void = {
    for (var k = 0; k < 1000000; k += 1) {
    };
};

fn main() void or error = {
    var out = int[8] {};
    var i = 0;
    par for (i = 0; i < 8; i += 1) {
        spin();
        out[i] = i * 10;
    };
    fmt.Println(i, out[0], out[3], out[7]) or return;
};
`,
	})
	stdout, stderr, err := p.run(t)
	if err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr)
	}
	expect(t, stdout, "8 0 30 70\n")
}

func TestParMapError(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use errors;

fn check(k: i32) i32 or error = {
    if (k == 1) {
        return 0, errors.New("key one");
    };
    if (k == 2) {
        return 0, errors.New("key two");
    };
    return k;
};

fn main() void or error = {
    var m = map[i32, i32] { 1 = 1, 2 = 2 };
    var out = i32[8] {};
    par for (var k in m) {
        out[k] = check(k) or return;
    };
    var dummy = out[0] + out[1] + out[2];
    if (dummy < 0) {
        return errors.New("unreachable");
    };
};
`,
	})
	src := p.source(t, "zz_strizh.go")
	contains(t, src, ".Add(-1, ")
	stdout, stderr, err := p.run(t)
	if err == nil {
		t.Fatalf("expected failure, stdout:\n%s", stdout)
	}
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "key ") {
		t.Fatalf("stderr does not contain a map iteration error: %s", stderr)
	}
}
