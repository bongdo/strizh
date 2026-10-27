package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/parser"
)

func TestConformanceCases(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join("..", "..", "tests", "cases")
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Skip(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		parsed := make(map[string][]*ast.File)
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".str") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			f, errs := parser.Parse(path, src)
			if len(errs) > 0 {
				t.Errorf("%s: %v", path, errs)
				return nil
			}
			rel, _ := filepath.Rel(dir, filepath.Dir(path))
			key := filepath.ToSlash(rel)
			parsed[key] = append(parsed[key], f)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		p := Check(parsed, Options{Root: root, Module: "example.com/test"})
		for _, e := range p.Errors {
			t.Errorf("%s: %s", entry.Name(), e.String())
		}
	}
}
