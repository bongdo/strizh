package sema

import "testing"

func TestLambdaInference(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `use sort;

type User = struct {
    age: i32,
};

fn main() void = {
    var users = User[] {
        User { age = 30 },
        User { age = 20 },
    };
    sort.Slice(users, fn(i, j) => users[i].age < users[j].age);
};
`})
	noErrors(t, p)
}

func TestMapCommaOK(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `fn main() void = {
    var m = map[str, i32] { "a" = 1 };
    var v, ok = m["a"];
    _ = v;
    _ = ok;
};
`})
	noErrors(t, p)
}

func TestMissingImportUsedInType(t *testing.T) {
	p := checkSrc(t, map[string]string{
		".": `use "./lib" as lib;

fn main() void = {
    var r = lib.Result { value = 1 };
};
`,
		"lib": `type Result = struct {
    Value: i32,
};

fn Result() void = {
};
`,
	})
	if len(p.Errors) == 0 {
		t.Fatal("expected an error for lowercase field or duplicate name")
	}
}

func TestUnusedImport(t *testing.T) {
	p := checkSrc(t, map[string]string{".": `use fmt;

fn main() void = {
};
`})
	found := false
	for _, e := range p.Errors {
		if e.Msg == "import fmt is not used" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unused import error, got %v", p.Errors)
	}
}
