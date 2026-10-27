package driver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hello")
	if err := New(dir); err != nil {
		t.Fatal(err)
	}
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "hello")
	if err := p.Build(out, nil); err != nil {
		p.PrintErrors(os.Stderr)
		t.Fatal(err)
	}
	got, err := exec.Command(out).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello from hello\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckTypeErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "strizh.toml"), []byte("name = \"tc\"\nmodule = \"example.com/tc\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	main := "fn main() void = {\n    var n = 1 + 2;\n    assert(n == 3);\n};\n"
	if err := os.WriteFile(filepath.Join(dir, "main.str"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := "fn Square(x: i32) i32 = {\n    return x + \"oops\";\n};\n"
	libPath := filepath.Join(dir, "lib", "lib.str")
	if err := os.WriteFile(libPath, []byte(lib), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Check(CheckOptions{Typecheck: true}); err != nil {
		t.Fatal(err)
	}
	if len(p.Errors) != 1 {
		t.Fatalf("errors: %+v", p.Errors)
	}
	got := p.Errors[0]
	if got.Path != libPath || !strings.Contains(got.Msg, "mismatched types") {
		t.Fatalf("error: %+v", got)
	}
	if got.Pos.Line != 2 {
		t.Fatalf("line: %+v", got.Pos)
	}
	for _, name := range []string{"zz_strizh.go", "lib/zz_strizh.go"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was written to the source tree", name)
		}
	}
}

func TestConformanceCases(t *testing.T) {
	base := filepath.Join("..", "..", "tests", "cases")
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Skip(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), entry.Name())
			copyTree(t, filepath.Join(base, entry.Name()), dir)
			p, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "out")
			if err := p.Build(out, nil); err != nil {
				p.PrintErrors(os.Stderr)
				t.Fatal(err)
			}
			if len(p.Errors) > 0 {
				p.PrintErrors(os.Stderr)
				t.Fatal("check failed")
			}
			cmd := exec.Command(out)
			cmd.Dir = dir
			var stderr strings.Builder
			cmd.Stderr = &stderr
			got, err := cmd.Output()
			if err != nil {
				t.Fatalf("%v\n%s", err, stderr.String())
			}
			expected, err := os.ReadFile(filepath.Join(dir, "expected.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(expected) {
				t.Fatalf("output mismatch:\n--- got ---\n%s--- want ---\n%s", got, expected)
			}
			if _, err := os.Stat(filepath.Join(dir, "test")); err == nil {
				if err := p.Generate(true); err != nil {
					t.Fatal(err)
				}
				testCmd := exec.Command("go", "test", "-buildvcs=false", "./...")
				testCmd.Dir = dir
				if output, err := testCmd.CombinedOutput(); err != nil {
					t.Fatalf("go test: %v\n%s", err, output)
				}
			}
		})
	}
}
