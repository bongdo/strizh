package gen

import (
	"strings"
	"testing"
)

func TestParDeferPanicWins(t *testing.T) {
	p := newProject(t, map[string]string{
		"main.str": `use errors;

fn boom() void = {
    panic("defer-panic");
};

fn fail() void or error = {
    return errors.New("original");
};

fn main() void or error = {
    par {
        {
            defer boom();
            fail() or return;
        };
    };
};
`,
	})
	_, stderr, err := p.run(t)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr, "defer-panic") {
		t.Fatalf("defer panic lost:\n%s", stderr)
	}
	if strings.Contains(stderr, "original") {
		t.Fatalf("original error should not win:\n%s", stderr)
	}
}
