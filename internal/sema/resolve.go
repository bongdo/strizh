package sema

import (
	"go/types"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
)

var builtinTypes = map[string]string{
	"void":     "",
	"bool":     "bool",
	"i8":       "int8",
	"i16":      "int16",
	"i32":      "int32",
	"i64":      "int64",
	"u8":       "uint8",
	"u16":      "uint16",
	"u32":      "uint32",
	"u64":      "uint64",
	"int":      "int",
	"uint":     "uint",
	"usize":    "uint",
	"isize":    "int",
	"f32":      "float32",
	"f64":      "float64",
	"str":      "string",
	"rune":     "rune",
	"byte":     "byte",
	"any":      "any",
	"error":    "error",
	"Duration": "time.Duration",
	"Type":     "reflect.Type",
}

func IsBuiltinType(name string) bool {
	_, ok := builtinTypes[name]
	return ok
}

func (p *Program) resolveTypeDecl(fc *fileCtx, d *ast.TypeDecl) {
	tparams := typeParamSet(d.TypeParams)
	if len(d.TypeParams) > 0 {
		seen := make(map[string]bool)
		for _, tp := range d.TypeParams {
			if !checkDeclName(p, fc.file, tp.Name, tp.Pos()) {
				continue
			}
			if seen[tp.Name] {
				p.errorf(fc.file.Path, tp.Pos(), "type parameter %s redeclared", tp.Name)
			}
			seen[tp.Name] = true
		}
	}
	switch d.Kind {
	case ast.TypeAlias:
		d.Target = p.resolveType(fc, d.Target, tparams)
		if d.Target != nil && isVoidType(d.Target) {
			p.errorf(fc.file.Path, d.Pos(), "alias %s cannot be void", d.Name)
		}
	case ast.TypeStruct:
		for _, f := range d.Fields {
			f.Type = p.resolveType(fc, f.Type, tparams)
			if f.Type != nil && isVoidType(f.Type) {
				p.errorf(fc.file.Path, f.Pos(), "field %s cannot be void", f.Name)
			}
		}
		for _, m := range d.Methods {
			if len(m.TypeParams) > 0 {
				p.errorf(fc.file.Path, m.Pos(), "methods cannot declare type parameters")
			}
			p.resolveSignature(fc, m, tparams)
		}
	case ast.TypeEnum:
		if len(d.TypeParams) > 0 {
			p.errorf(fc.file.Path, d.Pos(), "generic enums are not supported")
		}
		for _, v := range d.Variants {
			for _, f := range v.Fields {
				f.Type = p.resolveType(fc, f.Type, tparams)
				if f.Type != nil && isVoidType(f.Type) {
					p.errorf(fc.file.Path, f.Pos(), "field %s cannot be void", f.Name)
				}
			}
		}
	case ast.TypeInterface:
		for _, m := range d.IFace {
			p.resolveIfaceMethod(fc, m, tparams)
		}
	}
}

func typeParamSet(ps []*ast.TypeParam) map[string]bool {
	m := make(map[string]bool)
	for _, p := range ps {
		m[p.Name] = true
	}
	return m
}

func mergeParams(a, b map[string]bool) map[string]bool {
	if len(a) == 0 {
		return b
	}
	m := make(map[string]bool, len(a)+len(b))
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	return m
}

func (p *Program) resolveSignature(fc *fileCtx, fn *ast.FuncDecl, outer map[string]bool) {
	tparams := mergeParams(outer, typeParamSet(fn.TypeParams))
	seen := make(map[string]bool)
	for _, param := range fn.Params {
		if param.Name != "_" && !checkLocalName(p, fc.file, param.Name, param.Pos()) {
			continue
		}
		if param.Name != "_" && seen[param.Name] {
			p.errorf(fc.file.Path, param.Pos(), "parameter %s redeclared", param.Name)
		}
		seen[param.Name] = true
		param.Type = p.resolveType(fc, param.Type, tparams)
		if param.Type != nil && isVoidType(param.Type) {
			p.errorf(fc.file.Path, param.Pos(), "parameter %s cannot be void", param.Name)
		}
	}
	fn.Results = p.normalizeResults(fc, fn.Results, tparams)
	p.funcs[fn] = &funcInfo{decl: fn, results: fn.Results, err: fn.Err, file: fc}
}

func (p *Program) resolveIfaceMethod(fc *fileCtx, m *ast.IFaceMethod, outer map[string]bool) {
	seen := make(map[string]bool)
	for _, param := range m.Params {
		if param.Name != "_" && !checkLocalName(p, fc.file, param.Name, param.Pos()) {
			continue
		}
		if param.Name != "_" && seen[param.Name] {
			p.errorf(fc.file.Path, param.Pos(), "parameter %s redeclared", param.Name)
		}
		seen[param.Name] = true
		param.Type = p.resolveType(fc, param.Type, outer)
	}
	m.Results = p.normalizeResults(fc, m.Results, outer)
}

func (p *Program) normalizeResults(fc *fileCtx, ts []ast.Type, tparams map[string]bool) []ast.Type {
	var out []ast.Type
	for _, t := range ts {
		if r := p.normalizeResult(fc, t, tparams); r != nil {
			out = append(out, r)
		}
	}
	return out
}

func (p *Program) normalizeResult(fc *fileCtx, t ast.Type, tparams map[string]bool) ast.Type {
	if t == nil {
		return nil
	}
	t = p.resolveType(fc, t, tparams)
	if t == nil {
		return nil
	}
	if isVoidType(t) {
		return nil
	}
	return t
}

func (p *Program) resolveType(fc *fileCtx, t ast.Type, tparams map[string]bool) ast.Type {
	if t == nil {
		return nil
	}
	switch t := t.(type) {
	case *ast.GoType:
		return t
	case *ast.NamedType:
		return p.resolveNamed(fc, t, tparams)
	case *ast.SliceType:
		t.Elem = p.resolveType(fc, t.Elem, tparams)
		return t
	case *ast.ArrayType:
		t.Elem = p.resolveType(fc, t.Elem, tparams)
		t.Len = p.resolveConstExpr(fc, t.Len)
		return t
	case *ast.MapType:
		t.Key = p.resolveType(fc, t.Key, tparams)
		t.Value = p.resolveType(fc, t.Value, tparams)
		return t
	case *ast.PointerType:
		t.Elem = p.resolveType(fc, t.Elem, tparams)
		return t
	case *ast.FuncType:
		for i, param := range t.Params {
			t.Params[i] = p.resolveType(fc, param, tparams)
		}
		t.Results = p.normalizeResults(fc, t.Results, tparams)
		return t
	case *ast.InterfaceType:
		for _, m := range t.Methods {
			p.resolveIfaceMethod(fc, m, tparams)
		}
		return t
	case *ast.StructType:
		for _, f := range t.Fields {
			f.Type = p.resolveType(fc, f.Type, tparams)
		}
		return t
	case *ast.GenericType:
		return p.resolveGeneric(fc, t, tparams)
	}
	return t
}

func (p *Program) resolveNamed(fc *fileCtx, t *ast.NamedType, tparams map[string]bool) *ast.NamedType {
	if tparams[t.Name] && t.Package == "" {
		return t
	}
	if t.Package == "" {
		if IsBuiltinType(t.Name) {
			t.GoName = builtinTypes[t.Name]
			return t
		}
		if d, ok := fc.pkg.Types[t.Name]; ok {
			t.Decl = d
			t.GoName = d.GoName
			return t
		}
		p.errorf(fc.file.Path, t.Pos(), "unknown type %s", t.Name)
		return t
	}
	if imp, ok := fc.imports[t.Package]; ok {
		fc.mark(t.Package)
		if imp.Local != nil {
			d, ok := imp.Local.Types[t.Name]
			if !ok {
				p.errorf(fc.file.Path, t.Pos(), "package %s has no type %s", t.Package, t.Name)
				return t
			}
			if !exported(t.Name) && imp.Local != fc.pkg {
				p.errorf(fc.file.Path, t.Pos(), "type %s is not exported by %s", t.Name, t.Package)
				return t
			}
			t.Decl = d
			t.GoName = d.GoName
			t.PkgAlias = t.Package
			return t
		}
		if imp.Go != nil && imp.Go.Types != nil {
			obj := imp.Go.Types.Scope().Lookup(t.Name)
			if obj == nil {
				p.errorf(fc.file.Path, t.Pos(), "package %s has no type %s", t.Package, t.Name)
				return t
			}
			if _, ok := obj.(*types.TypeName); !ok {
				p.errorf(fc.file.Path, t.Pos(), "%s.%s is not a type", t.Package, t.Name)
				return t
			}
			if !obj.Exported() {
				p.errorf(fc.file.Path, t.Pos(), "type %s is not exported by %s", t.Name, t.Package)
				return t
			}
			t.GoName = t.Name
			t.PkgAlias = t.Package
			p.goNamed[t] = obj.Type()
			return t
		}
		p.errorf(fc.file.Path, t.Pos(), "unknown package %s", t.Package)
		return t
	}
	if d, ok := fc.pkg.Types[t.Package]; ok && d.Kind == ast.TypeEnum {
		v := findVariant(d, t.Name)
		if v == nil {
			p.errorf(fc.file.Path, t.Pos(), "enum %s has no variant %s", t.Package, t.Name)
			return t
		}
		t.Decl = d
		t.Variant = v
		t.GoName = v.GoName
		return t
	}
	if i := strings.LastIndexByte(t.Package, '.'); i >= 0 {
		alias := t.Package[:i]
		enumName := t.Package[i+1:]
		if imp, ok := fc.imports[alias]; ok && imp.Local != nil {
			fc.mark(alias)
			d, ok := imp.Local.Types[enumName]
			if !ok || d.Kind != ast.TypeEnum {
				p.errorf(fc.file.Path, t.Pos(), "package %s has no enum %s", alias, enumName)
				return t
			}
			if !exported(enumName) && imp.Local != fc.pkg {
				p.errorf(fc.file.Path, t.Pos(), "enum %s is not exported by %s", enumName, alias)
				return t
			}
			v := findVariant(d, t.Name)
			if v == nil {
				p.errorf(fc.file.Path, t.Pos(), "enum %s has no variant %s", enumName, t.Name)
				return t
			}
			t.Decl = d
			t.Variant = v
			t.GoName = v.GoName
			t.PkgAlias = alias
			return t
		}
	}
	p.errorf(fc.file.Path, t.Pos(), "unknown type %s.%s", t.Package, t.Name)
	return t
}

func (fc *fileCtx) mark(alias string) {
	if u := fc.uses[alias]; u != nil {
		u.Used = true
	}
}

func findVariant(d *ast.TypeDecl, name string) *ast.Variant {
	for _, v := range d.Variants {
		if v.Name == name {
			return v
		}
	}
	return nil
}

func (p *Program) resolveGeneric(fc *fileCtx, t *ast.GenericType, tparams map[string]bool) ast.Type {
	base := p.resolveType(fc, t.Base, tparams)
	t.Base = base
	if named, ok := base.(*ast.NamedType); ok {
		if named.Decl != nil && len(named.Decl.TypeParams) > 0 {
			if len(t.Args) != len(named.Decl.TypeParams) {
				p.errorf(fc.file.Path, t.Pos(), "type %s wants %d type arguments, got %d", named.Name, len(named.Decl.TypeParams), len(t.Args))
			}
			for i, a := range t.Args {
				t.Args[i] = p.resolveType(fc, a, tparams)
			}
			return t
		}
		if named.Decl == nil && named.PkgAlias != "" {
			return t
		}
		if IsBuiltinType(named.Name) && len(t.Args) == 1 {
			if lit := constExprOf(t.Args[0]); lit != nil {
				e := p.resolveConstExpr(fc, lit)
				return &ast.ArrayType{Span: t.Span, Elem: base, Len: e}
			}
		}
	}
	p.errorf(fc.file.Path, t.Pos(), "type is not generic")
	return t
}

func constExprOf(t ast.Type) ast.Expr {
	if n, ok := t.(*ast.NamedType); ok && n.Package == "" {
		return &ast.Ident{Span: n.Span, Name: n.Name}
	}
	return nil
}

func (p *Program) resolveConstExpr(fc *fileCtx, e ast.Expr) ast.Expr {
	if e == nil {
		return nil
	}
	switch e := e.(type) {
	case *ast.Ident:
		d, ok := fc.pkg.Vars[e.Name]
		if !ok || !d.Const {
			p.errorf(fc.file.Path, e.Pos(), "%s is not a constant", e.Name)
			return e
		}
		e.Ref = &ast.Ref{Kind: ast.RefConst, Name: e.Name, Decl: d, Path: fc.file.Path}
		return e
	case *ast.BasicLit:
		return e
	case *ast.UnaryExpr:
		e.X = p.resolveConstExpr(fc, e.X)
		return e
	case *ast.BinaryExpr:
		e.X = p.resolveConstExpr(fc, e.X)
		e.Y = p.resolveConstExpr(fc, e.Y)
		return e
	case *ast.ParenExpr:
		e.X = p.resolveConstExpr(fc, e.X)
		return e
	case *ast.CallExpr:
		for i, a := range e.Args {
			e.Args[i] = p.resolveConstExpr(fc, a)
		}
		return e
	}
	p.errorf(fc.file.Path, e.Pos(), "array length must be a constant expression")
	return e
}

func isVoidType(t ast.Type) bool {
	n, ok := t.(*ast.NamedType)
	return ok && n.Package == "" && n.Name == "void"
}

func exported(name string) bool {
	for _, r := range name {
		return r >= 'A' && r <= 'Z'
	}
	return false
}

func (p *Program) finalChecks() {
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			for _, u := range f.Uses {
				if !u.Used {
					p.errorf(f.Path, u.Pos(), "import %s is not used", u.Alias)
				}
			}
		}
	}
}

func (p *Program) checkMain() {
	if p.Main == nil {
		return
	}
	main := p.Main.Funcs["main"]
	if main == nil {
		p.errorf(p.Main.Files[0].Path, p.Main.Files[0].Pos(), "package main has no fn main")
		return
	}
	if len(main.Params) != 0 {
		p.errorf(p.Main.Files[0].Path, main.Pos(), "main takes no parameters")
	}
	if len(main.Results) > 0 {
		p.errorf(p.Main.Files[0].Path, main.Pos(), "main returns void or void or error")
	}
}

func (p *Program) checkPackageVars() {
	for _, pkg := range p.Packages {
		p.global[pkg] = make(map[string]ast.Type)
		for name := range pkg.Vars {
			p.global[pkg][name] = nil
		}
		for _, f := range pkg.Files {
			fc := p.fileCtx(pkg, f)
			for _, d := range f.Decls {
				v, ok := d.(*ast.VarDecl)
				if !ok {
					continue
				}
				c := &checker{p: p, fc: fc}
				for _, val := range v.Values {
					if _, ok := val.(*ast.OrExpr); ok {
						p.errorf(f.Path, val.Pos(), "or is not allowed in a package-level initializer")
					}
					c.resolveExpr(val, true)
				}
				var ts []ast.Type
				for _, val := range v.Values {
					ts = append(ts, c.exprType(val))
				}
				for i, name := range v.Names {
					if name == "_" {
						continue
					}
					var t ast.Type
					if len(ts) == len(v.Names) {
						t = ts[i]
					}
					p.global[pkg][name] = t
				}
			}
		}
	}
}
