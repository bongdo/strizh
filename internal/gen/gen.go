package gen

import (
	"fmt"
	"go/format"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/sema"
)

type File struct {
	Path string
	Src  []byte
}

type Options struct {
	Root         string
	Module       string
	IncludeTests bool
}

type generator struct {
	p      *sema.Program
	opt    Options
	fileOf map[ast.Node]string
	errs   []error
}

func Generate(p *sema.Program, files map[string][]*ast.File, opt Options) ([]File, []error) {
	if p == nil {
		return nil, []error{fmt.Errorf("gen: nil program")}
	}
	if opt.Root == "" {
		opt.Root = p.Root
	}
	if opt.Module == "" {
		opt.Module = p.Module
	}
	g := &generator{p: p, opt: opt, fileOf: make(map[ast.Node]string)}
	for _, fs := range files {
		for _, f := range fs {
			g.indexFile(f)
		}
	}
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			g.indexFile(f)
		}
	}
	var rels []string
	for rel := range p.Packages {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	var out []File
	for _, rel := range rels {
		pkg := p.Packages[rel]
		if pkg == nil {
			continue
		}
		src, err := g.genFile(pkg, false)
		if err != nil {
			g.errs = append(g.errs, err)
		} else {
			out = append(out, File{Path: g.filePath(pkg, false), Src: src})
		}
		if opt.IncludeTests && hasTests(pkg) {
			src, err := g.genFile(pkg, true)
			if err != nil {
				g.errs = append(g.errs, err)
			} else {
				out = append(out, File{Path: g.filePath(pkg, true), Src: src})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, g.errs
}

func (g *generator) indexFile(f *ast.File) {
	if f == nil {
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		if _, ok := g.fileOf[n]; !ok {
			g.fileOf[n] = f.Path
		}
		return true
	})
}

func (g *generator) filePath(pkg *sema.Package, test bool) string {
	dir := pkg.Dir
	if dir == "" {
		dir = filepath.Join(g.opt.Root, filepath.FromSlash(pkg.Rel))
	}
	name := "zz_strizh.go"
	if test {
		name = "zz_strizh_test.go"
	}
	return filepath.Join(dir, name)
}

func (g *generator) genFile(pkg *sema.Package, tests bool) ([]byte, error) {
	f := &fileGen{
		g:    g,
		pkg:  pkg,
		need: make(map[string]bool),
		used: make(map[string]bool),
	}
	for _, d := range pkg.Decls {
		if _, ok := d.(*ast.TestDecl); ok {
			continue
		}
		if tests {
			continue
		}
		w := newWriter(f, "_s_context.Background()")
		g.decl(w, d)
		f.body.WriteString(w.buf.String())
	}
	if tests {
		for _, file := range pkg.Files {
			for _, d := range file.Decls {
				t, ok := d.(*ast.TestDecl)
				if !ok {
					continue
				}
				w := newWriter(f, "_s_context.Background()")
				g.testDecl(w, t)
				f.body.WriteString(w.buf.String())
			}
		}
	}
	return f.assemble(g.filePath(pkg, tests))
}

func hasTests(pkg *sema.Package) bool {
	for _, file := range pkg.Files {
		for _, d := range file.Decls {
			if _, ok := d.(*ast.TestDecl); ok {
				return true
			}
		}
	}
	return false
}

func (g *generator) errAt(n ast.Node, format string, args ...any) {
	pos := ""
	if n != nil {
		pos = g.pos(n) + ": "
	}
	g.errs = append(g.errs, fmt.Errorf("%s%s", pos, fmt.Sprintf(format, args...)))
}

func (g *generator) pos(n ast.Node) string {
	if n == nil {
		return ""
	}
	path := g.fileOf[n]
	if path == "" {
		path = g.opt.Root
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(g.opt.Root, path)
	}
	line := n.Pos().Line
	if line < 1 {
		line = 1
	}
	return path + ":" + strconv.Itoa(line)
}

func (g *generator) writeLineDirective(w *writer, n ast.Node) {
	if n == nil {
		return
	}
	path := g.fileOf[n]
	if path == "" {
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(g.opt.Root, path)
	}
	line := n.Pos().Line
	if line < 1 {
		line = 1
	}
	w.buf.WriteString("//line ")
	w.buf.WriteString(path)
	w.buf.WriteByte(':')
	w.buf.WriteString(strconv.Itoa(line))
	w.buf.WriteByte('\n')
}

type fileGen struct {
	g    *generator
	pkg  *sema.Package
	body strings.Builder
	need map[string]bool
	used map[string]bool
}

func (f *fileGen) needPkg(alias string) {
	f.need[alias] = true
}

func (f *fileGen) usePkg(alias string) {
	if alias != "" {
		f.used[alias] = true
	}
}

var stdImports = map[string]string{
	"context": "context",
	"fmt":     "fmt",
	"os":      "os",
	"sync":    "sync",
	"runtime": "runtime",
	"time":    "time",
	"reflect": "reflect",
	"testing": "testing",
}

type importSpec struct {
	alias string
	path  string
}

func (f *fileGen) module() string {
	module := f.g.opt.Module
	if module == "" {
		module = f.g.p.Module
	}
	return strings.TrimSuffix(module, "/")
}

func (f *fileGen) runtimePath() string {
	return f.module() + "/.strizh/runtime"
}

func (f *fileGen) assemble(path string) ([]byte, error) {
	specs := make(map[string]importSpec)
	for alias := range f.need {
		if alias == "rt" {
			specs["_s_rt"] = importSpec{alias: "_s_rt", path: f.runtimePath()}
			continue
		}
		p, ok := stdImports[alias]
		if !ok {
			continue
		}
		specs["_s_"+alias] = importSpec{alias: "_s_" + alias, path: p}
	}
	for alias := range f.used {
		imp := f.pkg.Imports[alias]
		if imp == nil {
			continue
		}
		p := imp.Path
		if imp.Local != nil {
			base := f.module()
			if imp.Local.Rel == "." {
				p = base
			} else {
				p = base + "/" + imp.Local.Rel
			}
		}
		specs[alias] = importSpec{alias: alias, path: p}
	}
	var list []importSpec
	for _, s := range specs {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].path == list[j].path {
			return list[i].alias < list[j].alias
		}
		return list[i].path < list[j].path
	})
	var b strings.Builder
	b.WriteString("// Code generated by strizh; DO NOT EDIT.\n\n")
	b.WriteString("package ")
	b.WriteString(f.pkg.Name)
	b.WriteString("\n\n")
	if len(list) > 0 {
		b.WriteString("import (\n")
		for _, s := range list {
			b.WriteByte('\t')
			b.WriteString(s.alias)
			b.WriteByte(' ')
			b.WriteString(strconv.Quote(s.path))
			b.WriteByte('\n')
		}
		b.WriteString(")\n\n")
	}
	b.WriteString(f.body.String())
	src := []byte(b.String())
	out, err := format.Source(src)
	if err != nil {
		snippet := string(src)
		if len(snippet) > 4000 {
			snippet = snippet[:4000] + "\n..."
		}
		return nil, fmt.Errorf("%s: format: %v\n%s", path, err, snippet)
	}
	return out, nil
}

type writer struct {
	f          *fileGen
	buf        strings.Builder
	indent     int
	ctx        string
	n          *int
	deferStack bool
	loops      []string
	par        *parRef
	results    []ast.Type
	err        bool
	renames    map[string]rename
	self       string
}

type rename struct {
	name string
	decl ast.Node
}

type parRef struct {
	pe     string
	ord    string
	cancel string
}

func newWriter(f *fileGen, ctx string) *writer {
	n := 0
	return &writer{f: f, ctx: ctx, n: &n, renames: make(map[string]rename)}
}

func (w *writer) next(prefix string) string {
	if w.n == nil {
		n := 0
		w.n = &n
	}
	*w.n = *w.n + 1
	return "_s_" + prefix + strconv.Itoa(*w.n)
}

func (w *writer) line(s string) {
	w.writeIndent()
	w.buf.WriteString(s)
	w.buf.WriteByte('\n')
}

func (w *writer) linef(format string, args ...any) {
	w.line(fmt.Sprintf(format, args...))
}

func (w *writer) writeIndent() {
	for i := 0; i < w.indent; i++ {
		w.buf.WriteByte('\t')
	}
}

func (w *writer) directive(n ast.Node) {
	w.f.g.writeLineDirective(w, n)
}

func (w *writer) sub() *writer {
	sub := &writer{
		f:       w.f,
		ctx:     w.ctx,
		n:       w.n,
		results: w.results,
		err:     w.err,
		renames: make(map[string]rename),
		self:    w.self,
	}
	for k, v := range w.renames {
		sub.renames[k] = v
	}
	return sub
}

func (w *writer) currentCtx() string {
	if w.ctx == "" {
		w.ctx = "_s_context.Background()"
	}
	if w.ctx == "_s_context.Background()" {
		w.f.needPkg("context")
	}
	return w.ctx
}

func (w *writer) typeStr(t ast.Type) string {
	return w.f.g.typeStr(w.f, t)
}

func (w *writer) selfRef() string {
	if w.self == "" {
		return "self"
	}
	return w.self
}

func joinExprs(xs []string) string {
	return strings.Join(xs, ", ")
}
