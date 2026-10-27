package driver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/config"
	"github.com/bongdo/strizh/internal/format"
	"github.com/bongdo/strizh/internal/gen"
	"github.com/bongdo/strizh/internal/parser"
	"github.com/bongdo/strizh/internal/sema"
	"github.com/bongdo/strizh/internal/token"
	"github.com/bongdo/strizh/internal/typecheck"
	strizhrt "github.com/bongdo/strizh/runtime"
)

const Version = "0.1.0"

var ErrCheck = errors.New("check failed")

type Error struct {
	Path string
	Pos  token.Pos
	Msg  string
}

func (e Error) String() string {
	if e.Pos.Valid() {
		return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Pos.Line, e.Pos.Col, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Msg)
}

type Project struct {
	Root    string
	Config  *config.Config
	Module  string
	Parsed  map[string][]*ast.File
	Program *sema.Program
	Errors  []Error
	Stub    string
}

type CheckOptions struct {
	Tidy      bool
	Typecheck bool
}

func Open(dir string) (*Project, error) {
	root, err := config.FindRoot(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	p := &Project{Root: root, Config: cfg, Module: cfg.Module}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Project) parse() error {
	p.Parsed = make(map[string][]*ast.File)
	var errs []Error
	err := filepath.WalkDir(p.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == p.Root {
				return nil
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".str") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, Error{Path: path, Msg: err.Error()})
			return nil
		}
		f, perrs := parser.Parse(path, src)
		for _, e := range perrs {
			errs = append(errs, Error{Path: path, Pos: e.Pos, Msg: e.Msg})
		}
		if f == nil {
			return nil
		}
		rel := p.relDir(filepath.Dir(path))
		p.Parsed[rel] = append(p.Parsed[rel], f)
		return nil
	})
	if err != nil {
		return err
	}
	for _, files := range p.Parsed {
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	}
	p.Errors = errs
	return nil
}

func (p *Project) relDir(dir string) string {
	rel, err := filepath.Rel(p.Root, dir)
	if err != nil {
		return filepath.ToSlash(dir)
	}
	return filepath.ToSlash(rel)
}

func (p *Project) GoImports() []string {
	seen := make(map[string]bool)
	var out []string
	for _, files := range p.Parsed {
		for _, f := range files {
			for _, u := range f.Uses {
				if u.Local || u.Path == "" || seen[u.Path] {
					continue
				}
				seen[u.Path] = true
				out = append(out, u.Path)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (p *Project) Check(opt CheckOptions) error {
	p.Errors = nil
	if err := p.parse(); err != nil {
		return err
	}
	if len(p.Parsed) == 0 {
		p.Errors = append(p.Errors, Error{Path: p.Root, Msg: "no .str files"})
		return nil
	}
	if len(p.Errors) > 0 {
		return nil
	}
	if err := p.ensureModule(); err != nil {
		return err
	}
	imports := p.GoImports()
	if err := p.withImportStub(imports, opt.Tidy, func() error {
		p.Program = sema.Check(p.Parsed, sema.Options{Root: p.Root, Module: p.Module})
		for _, e := range p.Program.Errors {
			p.Errors = append(p.Errors, Error{Path: e.Path, Pos: e.Pos, Msg: e.Msg})
		}
		return nil
	}); err != nil {
		return err
	}
	if opt.Typecheck && len(p.Errors) == 0 {
		for _, e := range typecheck.Check(p.Program, p.Parsed, typecheck.Options{Root: p.Root, Module: p.Module}) {
			p.Errors = append(p.Errors, Error{Path: e.Path, Pos: e.Pos, Msg: e.Msg})
		}
	}
	return nil
}

func (p *Project) ensureModule() error {
	gomod := filepath.Join(p.Root, "go.mod")
	if data, err := os.ReadFile(gomod); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "module ") {
				p.Module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				return nil
			}
		}
		return errors.New("go.mod has no module line")
	} else if !os.IsNotExist(err) {
		return err
	}
	content := fmt.Sprintf("module %s\n\ngo %s\n", p.Config.Module, p.Config.Go)
	if err := os.WriteFile(gomod, []byte(content), 0o644); err != nil {
		return err
	}
	p.Module = p.Config.Module
	return nil
}

func (p *Project) withImportStub(imports []string, tidy bool, fn func() error) error {
	if len(imports) == 0 {
		return fn()
	}
	if !needsStub(imports) {
		return fn()
	}
	stub := filepath.Join(p.Root, "zz_strizh_imports.go")
	var b strings.Builder
	b.WriteString("package main\n\nimport (\n")
	for _, path := range imports {
		fmt.Fprintf(&b, "\t_ %q\n", path)
	}
	b.WriteString(")\n")
	if err := os.WriteFile(stub, []byte(b.String()), 0o644); err != nil {
		return err
	}
	p.Stub = stub
	defer func() {
		os.Remove(stub)
		p.Stub = ""
	}()
	if tidy && p.needsTidy(imports) {
		if out, err := p.goCommand("mod", "tidy"); err != nil {
			return fmt.Errorf("go mod tidy: %v\n%s", err, out)
		}
	}
	return fn()
}

func needsStub(imports []string) bool {
	for _, path := range imports {
		if strings.Contains(firstSegment(path), ".") {
			return true
		}
	}
	return false
}

func (p *Project) needsTidy(imports []string) bool {
	gomod, err := os.ReadFile(filepath.Join(p.Root, "go.mod"))
	if err != nil {
		return true
	}
	_, sumErr := os.Stat(filepath.Join(p.Root, "go.sum"))
	for _, path := range imports {
		if !strings.Contains(firstSegment(path), ".") {
			continue
		}
		prefix := modulePrefix(path)
		if !strings.Contains(string(gomod), prefix) {
			return true
		}
		if sumErr != nil {
			return true
		}
	}
	return false
}

func firstSegment(path string) string {
	if i := strings.IndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return path
}

func modulePrefix(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, "/")
}

func (p *Project) goCommand(args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = p.Root
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (p *Project) Generate(tests bool) error {
	if p.Program == nil {
		return errors.New("project is not checked")
	}
	files, errs := gen.Generate(p.Program, p.Parsed, gen.Options{Root: p.Root, Module: p.Module, IncludeTests: tests})
	if len(errs) > 0 {
		for _, e := range errs {
			p.Errors = append(p.Errors, Error{Path: p.Root, Msg: e.Error()})
		}
		return errs[0]
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.Path, f.Src, 0o644); err != nil {
			return err
		}
	}
	if tests {
		for _, pkg := range p.Program.Packages {
			if !pkgHasTests(pkg) {
				_ = os.Remove(filepath.Join(pkg.Dir, "zz_strizh_test.go"))
			}
		}
	}
	return p.writeRuntime()
}

func pkgHasTests(pkg *sema.Package) bool {
	for _, f := range pkg.Files {
		for _, d := range f.Decls {
			if _, ok := d.(*ast.TestDecl); ok {
				return true
			}
		}
	}
	return false
}

func (p *Project) writeRuntime() error {
	dir := filepath.Join(p.Root, ".strizh", "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "runtime.go"), strizhrt.Source, 0o644)
}

func (p *Project) Build(out string, goArgs []string) error {
	if err := p.Check(CheckOptions{Tidy: true}); err != nil {
		return err
	}
	if len(p.Errors) > 0 {
		return ErrCheck
	}
	if err := p.Generate(false); err != nil {
		return err
	}
	if out == "" {
		out = filepath.Join("bin", p.Config.Name)
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(p.Root, out)
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		out = filepath.Join(out, p.Config.Name)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	args := append([]string{"build", "-buildvcs=false"}, goArgs...)
	args = append(args, "-o", out, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir = p.Root
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed")
	}
	return nil
}

func (p *Project) Run(args []string) (int, error) {
	out := filepath.Join(p.Root, ".strizh", "bin", p.Config.Name)
	if err := p.Build(out, nil); err != nil {
		return 1, err
	}
	if len(p.Errors) > 0 {
		return 1, ErrCheck
	}
	return execRun(out, p.Root, args)
}

func execRun(bin, dir string, args []string) (int, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 1, err
}

func (p *Project) Test(args []string) (int, error) {
	if err := p.Check(CheckOptions{Tidy: true}); err != nil {
		return 1, err
	}
	if len(p.Errors) > 0 {
		return 1, ErrCheck
	}
	if err := p.Generate(true); err != nil {
		return 1, err
	}
	cmd := exec.Command("go", append([]string{"test", "-buildvcs=false", "./..."}, args...)...)
	cmd.Dir = p.Root
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 1, err
}

func (p *Project) Format(write bool, paths []string, stdout io.Writer) error {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	var files []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != path && (strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") || d.Name() == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), ".str") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	sort.Strings(files)
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, errs := parser.Parse(path, src)
		if len(errs) > 0 {
			return fmt.Errorf("%s:%d:%d: %s", path, errs[0].Pos.Line, errs[0].Pos.Col, errs[0].Msg)
		}
		out := format.File(f)
		if write {
			if err := os.WriteFile(path, out, 0o644); err != nil {
				return err
			}
			continue
		}
		if stdout != nil {
			if _, err := stdout.Write(out); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Project) PrintErrors(w io.Writer) {
	for _, e := range p.Errors {
		fmt.Fprintln(w, e.String())
	}
}

func New(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty", dir)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	name := filepath.Base(abs)
	cfg := &config.Config{Name: name, Module: "example.com/" + name, Go: "1.27"}
	if err := config.Save(abs, cfg); err != nil {
		return err
	}
	main := fmt.Sprintf("use fmt;\n\nfn main() void or error = {\n    fmt.Println(%q) or return;\n};\n", "hello from "+name)
	if err := os.WriteFile(filepath.Join(abs, "main.str"), []byte(main), 0o644); err != nil {
		return err
	}
	gitignore := ".strizh/\nbin/\nzz_strizh.go\nzz_strizh_test.go\n"
	if err := os.WriteFile(filepath.Join(abs, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		return err
	}
	gomod := fmt.Sprintf("module %s\n\ngo %s\n", cfg.Module, cfg.Go)
	return os.WriteFile(filepath.Join(abs, "go.mod"), []byte(gomod), 0o644)
}

func Init(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(abs, config.FileName)); err == nil {
		return fmt.Errorf("%s already exists", config.FileName)
	}
	name := filepath.Base(abs)
	cfg := &config.Config{Name: name, Module: "example.com/" + name, Go: "1.27"}
	if err := config.Save(abs, cfg); err != nil {
		return err
	}
	main := filepath.Join(abs, "main.str")
	if _, err := os.Stat(main); os.IsNotExist(err) {
		src := "use fmt;\n\nfn main() void or error = {\n    fmt.Println(\"hello\") or return;\n};\n"
		return os.WriteFile(main, []byte(src), 0o644)
	}
	return nil
}
