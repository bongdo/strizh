package sema

import (
	"fmt"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/token"
	"golang.org/x/tools/go/packages"
)

type Error struct {
	Path    string
	Pos     token.Pos
	Msg     string
	Warning bool
}

func (e Error) String() string {
	if e.Pos.Valid() {
		return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Pos.Line, e.Pos.Col, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Msg)
}

type GoPkg struct {
	Path  string
	Name  string
	Types *types.Package
	Err   string
}

type Import struct {
	Alias string
	Path  string
	Local *Package
	Go    *GoPkg
}

type Package struct {
	Rel       string
	Dir       string
	Name      string
	IsMain    bool
	Files     []*ast.File
	Imports   map[string]*Import
	Types     map[string]*ast.TypeDecl
	Funcs     map[string]*ast.FuncDecl
	Vars      map[string]*ast.VarDecl
	Decls     []ast.Decl
	GoImports []string
}

type Program struct {
	Root      string
	Module    string
	Packages  map[string]*Package
	Main      *Package
	Errors    []Error
	GoTypes   map[ast.Expr]types.Type
	ExprTypes map[ast.Expr]ast.Type
	goNamed   map[*ast.NamedType]types.Type
	goOf      map[ast.Type]types.Type
	declPkg   map[*ast.TypeDecl]*Package
	global    map[*Package]map[string]ast.Type
	funcs     map[ast.Node]*funcInfo
	needsCtx  map[ast.Node]bool
	edges     []ctxEdge
	litCalls  []litCall
	goCache   map[string]*GoPkg
}

type Options struct {
	Root    string
	Module  string
	GoCache map[string]*GoPkg
}

type funcInfo struct {
	decl    *ast.FuncDecl
	lit     *ast.FuncLit
	test    *ast.TestDecl
	results []ast.Type
	err     bool
	file    *fileCtx
}

type fileCtx struct {
	file    *ast.File
	pkg     *Package
	imports map[string]*Import
	uses    map[string]*ast.Use
}

func Check(parsed map[string][]*ast.File, opt Options) *Program {
	p := &Program{
		Root:      opt.Root,
		Module:    opt.Module,
		Packages:  make(map[string]*Package),
		GoTypes:   make(map[ast.Expr]types.Type),
		ExprTypes: make(map[ast.Expr]ast.Type),
		goNamed:   make(map[*ast.NamedType]types.Type),
		goOf:      make(map[ast.Type]types.Type),
		declPkg:   make(map[*ast.TypeDecl]*Package),
		global:    make(map[*Package]map[string]ast.Type),
		funcs:     make(map[ast.Node]*funcInfo),
		needsCtx:  make(map[ast.Node]bool),
		goCache:   opt.GoCache,
	}
	for rel, files := range parsed {
		pkg := &Package{
			Rel:     rel,
			Dir:     filepath.Join(opt.Root, filepath.FromSlash(rel)),
			Name:    packageName(rel),
			IsMain:  rel == ".",
			Files:   files,
			Imports: make(map[string]*Import),
			Types:   make(map[string]*ast.TypeDecl),
			Funcs:   make(map[string]*ast.FuncDecl),
			Vars:    make(map[string]*ast.VarDecl),
		}
		sort.Slice(pkg.Files, func(i, j int) bool { return pkg.Files[i].Path < pkg.Files[j].Path })
		p.Packages[rel] = pkg
	}
	p.Main = p.Packages["."]
	p.collect()
	p.resolveUses()
	p.loadGo()
	p.bindImports()
	p.checkCycles()
	p.resolveDecls()
	p.checkPackageVars()
	p.checkFuncs()
	p.checkTests()
	p.checkMain()
	p.colorContext()
	p.finalChecks()
	return p
}

func packageName(rel string) string {
	if rel == "." {
		return "main"
	}
	base := path.Base(rel)
	var b strings.Builder
	for i, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if i == 0 && r >= '0' && r <= '9' {
			b.Reset()
			b.WriteByte('_')
			b.WriteRune(r)
		}
	}
	name := b.String()
	if name == "" {
		name = "pkg"
	}
	if token.GoReserved(name) {
		name += "_"
	}
	return name
}

func (p *Program) errorf(path string, pos token.Pos, format string, args ...any) {
	p.Errors = append(p.Errors, Error{Path: path, Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

func (p *Program) warnf(path string, pos token.Pos, format string, args ...any) {
	p.Errors = append(p.Errors, Error{Path: path, Pos: pos, Msg: fmt.Sprintf(format, args...), Warning: true})
}

func checkDeclName(p *Program, file *ast.File, name string, pos token.Pos) bool {
	if name == "" {
		p.errorf(file.Path, pos, "empty name")
		return false
	}
	if token.GoReserved(name) {
		p.errorf(file.Path, pos, "%q is a Go keyword", name)
		return false
	}
	if strings.HasPrefix(name, "_s_") {
		p.errorf(file.Path, pos, "names starting with _s_ are reserved")
		return false
	}
	return true
}

func checkTopName(p *Program, file *ast.File, name string, pos token.Pos) bool {
	if !checkDeclName(p, file, name, pos) {
		return false
	}
	if IsBuiltinType(name) || isBuiltinFunc(name) {
		p.errorf(file.Path, pos, "%s shadows a builtin", name)
		return false
	}
	return true
}

func checkLocalName(p *Program, file *ast.File, name string, pos token.Pos) bool {
	if name == "_" {
		return true
	}
	return checkDeclName(p, file, name, pos)
}

func (p *Program) collect() {
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				pkg.Decls = append(pkg.Decls, d)
				switch d := d.(type) {
				case *ast.TypeDecl:
					if !checkTopName(p, f, d.Name, d.Pos()) {
						continue
					}
					if _, ok := pkg.Types[d.Name]; ok {
						p.errorf(f.Path, d.Pos(), "type %s redeclared", d.Name)
						continue
					}
					if _, ok := pkg.Funcs[d.Name]; ok {
						p.errorf(f.Path, d.Pos(), "type %s conflicts with a function", d.Name)
					}
					pkg.Types[d.Name] = d
					d.GoName = d.Name
					p.declPkg[d] = pkg
					p.collectTypeMembers(pkg, f, d)
				case *ast.FuncDecl:
					if !checkTopName(p, f, d.Name, d.Pos()) {
						continue
					}
					if _, ok := pkg.Funcs[d.Name]; ok {
						p.errorf(f.Path, d.Pos(), "function %s redeclared", d.Name)
						continue
					}
					if _, ok := pkg.Types[d.Name]; ok {
						p.errorf(f.Path, d.Pos(), "function %s conflicts with a type", d.Name)
					}
					pkg.Funcs[d.Name] = d
					d.GoName = d.Name
				case *ast.VarDecl:
					for _, name := range d.Names {
						if name == "_" {
							continue
						}
						if !checkTopName(p, f, name, d.Pos()) {
							continue
						}
						if _, ok := pkg.Vars[name]; ok {
							p.errorf(f.Path, d.Pos(), "%s redeclared", name)
							continue
						}
						if _, ok := pkg.Funcs[name]; ok {
							p.errorf(f.Path, d.Pos(), "%s conflicts with a function", name)
						}
						if _, ok := pkg.Types[name]; ok {
							p.errorf(f.Path, d.Pos(), "%s conflicts with a type", name)
						}
						pkg.Vars[name] = d
					}
				}
			}
		}
	}
}

func (p *Program) collectTypeMembers(pkg *Package, f *ast.File, d *ast.TypeDecl) {
	switch d.Kind {
	case ast.TypeStruct:
		seenField := make(map[string]bool)
		for _, fld := range d.Fields {
			if !checkDeclName(p, f, fld.Name, fld.Pos()) {
				continue
			}
			if seenField[fld.Name] {
				p.errorf(f.Path, fld.Pos(), "field %s redeclared", fld.Name)
			}
			seenField[fld.Name] = true
		}
		seenMethod := make(map[string]bool)
		for _, m := range d.Methods {
			if !checkDeclName(p, f, m.Name, m.Pos()) {
				continue
			}
			m.Owner = d
			if seenMethod[m.Name] {
				p.errorf(f.Path, m.Pos(), "method %s redeclared", m.Name)
			}
			if seenField[m.Name] {
				p.errorf(f.Path, m.Pos(), "method %s conflicts with a field", m.Name)
			}
			seenMethod[m.Name] = true
		}
	case ast.TypeEnum:
		seen := make(map[string]bool)
		for _, v := range d.Variants {
			if !checkDeclName(p, f, v.Name, v.Pos()) {
				continue
			}
			if seen[v.Name] {
				p.errorf(f.Path, v.Pos(), "variant %s redeclared", v.Name)
			}
			seen[v.Name] = true
			v.Enum = d
			v.GoName = d.Name + v.Name
			seenF := make(map[string]bool)
			for _, fld := range v.Fields {
				if !checkDeclName(p, f, fld.Name, fld.Pos()) {
					continue
				}
				if seenF[fld.Name] {
					p.errorf(f.Path, fld.Pos(), "field %s redeclared", fld.Name)
				}
				seenF[fld.Name] = true
			}
			if _, ok := pkg.Types[v.GoName]; ok {
				p.errorf(f.Path, v.Pos(), "generated type %s conflicts with a declared type", v.GoName)
			}
		}
	case ast.TypeInterface:
		seen := make(map[string]bool)
		for _, m := range d.IFace {
			if !checkDeclName(p, f, m.Name, m.Pos()) {
				continue
			}
			if seen[m.Name] {
				p.errorf(f.Path, m.Pos(), "method %s redeclared", m.Name)
			}
			seen[m.Name] = true
		}
	}
}

func (p *Program) resolveUses() {
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			for _, u := range f.Uses {
				if u.Alias != "" && !checkDeclName(p, f, u.Alias, u.Pos()) {
					continue
				}
				if u.Local {
					target, ok := p.localImport(pkg, f, u.Path)
					if !ok {
						continue
					}
					u.LocalDir = target.Rel
					u.LocalPackage = target.Name
					if u.Alias == "" {
						u.Alias = target.Name
					}
					u.PackageName = target.Name
					u.GoPath = p.Module + "/" + target.Rel
					if target.Rel == "." {
						u.GoPath = p.Module
					}
					p.addImport(pkg, f, u, &Import{Alias: u.Alias, Path: u.Path, Local: target})
				} else {
					u.GoPath = u.Path
					if u.Alias == "" {
						u.Alias = path.Base(u.Path)
					}
					p.addImport(pkg, f, u, &Import{Alias: u.Alias, Path: u.Path})
				}
			}
		}
	}
}

func (p *Program) localImport(pkg *Package, f *ast.File, importPath string) (*Package, bool) {
	if !strings.HasPrefix(importPath, ".") {
		p.errorf(f.Path, f.Pos(), "local import must start with ./ or ../: %s", importPath)
		return nil, false
	}
	rel := path.Clean(path.Join(pkg.Rel, importPath))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		p.errorf(f.Path, f.Pos(), "import %s escapes the project", importPath)
		return nil, false
	}
	target, ok := p.Packages[rel]
	if !ok {
		p.errorf(f.Path, f.Pos(), "local package %s not found", importPath)
		return nil, false
	}
	return target, true
}

func (p *Program) addImport(pkg *Package, f *ast.File, u *ast.Use, imp *Import) {
	if prev, ok := pkg.Imports[imp.Alias]; ok {
		if prev.Path != imp.Path || prev.Local != imp.Local {
			p.errorf(f.Path, u.Pos(), "import alias %s used for both %s and %s", imp.Alias, prev.Path, imp.Path)
		}
		return
	}
	for name, decl := range pkg.Types {
		if name == imp.Alias {
			p.errorf(f.Path, u.Pos(), "import %s conflicts with type %s", imp.Alias, decl.Name)
		}
	}
	for name := range pkg.Funcs {
		if name == imp.Alias {
			p.errorf(f.Path, u.Pos(), "import %s conflicts with a function", imp.Alias)
		}
	}
	for name := range pkg.Vars {
		if name == imp.Alias {
			p.errorf(f.Path, u.Pos(), "import %s conflicts with a variable", imp.Alias)
		}
	}
	pkg.Imports[imp.Alias] = imp
}

func (p *Program) loadGo() {
	var paths []string
	seen := make(map[string]bool)
	for _, pkg := range p.Packages {
		for _, imp := range pkg.Imports {
			if imp.Local == nil && !seen[imp.Path] {
				seen[imp.Path] = true
				paths = append(paths, imp.Path)
			}
		}
	}
	if len(paths) == 0 {
		return
	}
	if p.goCache == nil {
		p.goCache = make(map[string]*GoPkg)
	}
	var missing []string
	for _, path := range paths {
		if _, ok := p.goCache[path]; !ok {
			missing = append(missing, path)
		}
	}
	for _, pkg := range p.Packages {
		for _, imp := range pkg.Imports {
			if gp := p.goCache[imp.Path]; gp != nil && imp.Local == nil {
				imp.Go = gp
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	paths = missing
	sort.Strings(paths)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
		Dir:  p.Root,
		Env:  append(os.Environ(), "GOFLAGS=-buildvcs=false"),
	}
	loaded, err := packages.Load(cfg, paths...)
	if err != nil {
		for _, pkg := range p.Packages {
			for _, imp := range pkg.Imports {
				if imp.Local == nil {
					imp.Go = &GoPkg{Path: imp.Path, Err: err.Error()}
				}
			}
		}
		return
	}
	byPath := make(map[string]*GoPkg)
	for _, lp := range loaded {
		gp := &GoPkg{Path: lp.PkgPath, Name: lp.Name, Types: lp.Types}
		if len(lp.Errors) > 0 {
			gp.Err = lp.Errors[0].Msg
		}
		byPath[lp.PkgPath] = gp
	}
	for path, gp := range byPath {
		p.goCache[path] = gp
	}
	for _, pkg := range p.Packages {
		for _, imp := range pkg.Imports {
			if imp.Local != nil {
				continue
			}
			imp.Go = p.goCache[imp.Path]
			if imp.Go == nil {
				imp.Go = &GoPkg{Path: imp.Path, Err: "package not found"}
			}
		}
	}
}

func (p *Program) bindImports() {
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			for _, u := range f.Uses {
				imp := pkg.Imports[u.Alias]
				if imp == nil {
					continue
				}
				if imp.Go != nil {
					if imp.Go.Err != "" {
						p.errorf(f.Path, u.Pos(), "cannot load Go package %s: %s", imp.Path, imp.Go.Err)
						continue
					}
					u.PackageName = imp.Go.Name
				}
				pkg.GoImports = appendUnique(pkg.GoImports, imp.Path)
			}
		}
	}
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

func (p *Program) checkCycles() {
	const (
		white = iota
		gray
		black
	)
	color := make(map[*Package]int)
	var visit func(pkg *Package) bool
	var stack []string
	visit = func(pkg *Package) bool {
		color[pkg] = gray
		for _, imp := range pkg.Imports {
			if imp.Local == nil {
				continue
			}
			switch color[imp.Local] {
			case gray:
				p.errorf(pkg.Files[0].Path, pkg.Files[0].Pos(), "import cycle: %s", strings.Join(append(stack, imp.Local.Rel), " -> "))
				return false
			case white:
				stack = append(stack, pkg.Rel)
				if !visit(imp.Local) {
					return false
				}
				stack = stack[:len(stack)-1]
			}
		}
		color[pkg] = black
		return true
	}
	for _, pkg := range p.Packages {
		if color[pkg] == white {
			if !visit(pkg) {
				return
			}
		}
	}
}

func (p *Program) fileCtx(pkg *Package, f *ast.File) *fileCtx {
	ctx := &fileCtx{file: f, pkg: pkg, imports: make(map[string]*Import), uses: make(map[string]*ast.Use)}
	for _, u := range f.Uses {
		ctx.uses[u.Alias] = u
		if imp := pkg.Imports[u.Alias]; imp != nil {
			ctx.imports[u.Alias] = imp
		}
	}
	return ctx
}

func (p *Program) resolveDecls() {
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			fc := p.fileCtx(pkg, f)
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.TypeDecl:
					p.resolveTypeDecl(fc, d)
				case *ast.FuncDecl:
					p.resolveSignature(fc, d, nil)
				}
			}
		}
	}
}
