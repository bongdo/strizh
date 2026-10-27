package sema

import (
	"go/types"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/token"
)

func (c *checker) resolveExpr(e ast.Expr, top bool) ast.Type {
	if e == nil {
		return nil
	}
	t := c.resolveExprInner(e, top)
	if t != nil {
		c.p.ExprTypes[e] = t
	}
	return t
}

func (c *checker) resolveExprInner(e ast.Expr, top bool) ast.Type {
	switch e := e.(type) {
	case *ast.Ident:
		return c.resolveIdent(e)
	case *ast.BasicLit:
		return litType(e)
	case *ast.SelectorExpr:
		return c.resolveSelector(e)
	case *ast.CallExpr:
		return c.resolveCall(e)
	case *ast.IndexExpr:
		xt := c.resolveExpr(e.X, false)
		c.resolveExpr(e.Index, false)
		return elemType(xt)
	case *ast.SliceExpr:
		xt := c.resolveExpr(e.X, false)
		c.resolveExpr(e.Low, false)
		c.resolveExpr(e.High, false)
		return xt
	case *ast.UnaryExpr:
		xt := c.resolveExpr(e.X, false)
		switch e.Op {
		case token.Amp:
			if xt == nil {
				return nil
			}
			return &ast.PointerType{Span: e.Span, Elem: xt}
		case token.Star:
			return elemType(xt)
		case token.Bang:
			return &ast.NamedType{Name: "bool", GoName: "bool"}
		default:
			return xt
		}
	case *ast.BinaryExpr:
		if n, ok := e.X.(*ast.NilLit); ok && n.Type == nil && (e.Op == token.Eq || e.Op == token.Ne) {
			yt := c.resolveExpr(e.Y, false)
			c.applyNil(n, yt)
			c.resolveExpr(e.X, false)
			return &ast.NamedType{Name: "bool", GoName: "bool"}
		}
		xt := c.resolveExpr(e.X, false)
		if n, ok := e.Y.(*ast.NilLit); ok && n.Type == nil && (e.Op == token.Eq || e.Op == token.Ne) {
			c.applyNil(n, xt)
		}
		yt := c.resolveExpr(e.Y, false)
		switch e.Op {
		case token.Eq, token.Ne, token.Lt, token.Le, token.Gt, token.Ge, token.AndAnd, token.OrOr:
			return &ast.NamedType{Name: "bool", GoName: "bool"}
		}
		if xt != nil {
			return xt
		}
		return yt
	case *ast.ParenExpr:
		return c.resolveExpr(e.X, top)
	case *ast.CompositeLit:
		return c.resolveComposite(e)
	case *ast.FuncLit:
		return c.resolveFuncLit(e, nil)
	case *ast.NilLit:
		if e.Type == nil {
			c.p.errorf(c.fc.file.Path, e.Pos(), "cannot infer type of nil; use nil[T] or an expected type")
			return nil
		}
		t := c.p.resolveType(c.fc, e.Type, c.tparams)
		e.Type = t
		if t != nil && !c.nilable(t) {
			c.p.errorf(c.fc.file.Path, e.Pos(), "nil is not valid for %s", typeName(t))
		}
		return t
	case *ast.MatchExpr:
		return c.resolveMatch(e)
	case *ast.OrExpr:
		return c.resolveOr(e, top)
	}
	return nil
}

func litType(e *ast.BasicLit) ast.Type {
	switch e.Kind {
	case token.Int:
		return &ast.NamedType{Span: e.Span, Name: "int", GoName: "int"}
	case token.Float:
		return &ast.NamedType{Span: e.Span, Name: "f64", GoName: "float64"}
	case token.String, token.RawString:
		return &ast.NamedType{Span: e.Span, Name: "str", GoName: "string"}
	case token.Rune:
		return &ast.NamedType{Span: e.Span, Name: "rune", GoName: "rune"}
	case token.Duration:
		return &ast.NamedType{Span: e.Span, Name: "Duration", GoName: "time.Duration"}
	case token.True, token.False:
		return &ast.NamedType{Span: e.Span, Name: "bool", GoName: "bool"}
	}
	return nil
}

func (c *checker) resolveIdent(id *ast.Ident) ast.Type {
	if id.Name == "_" {
		return nil
	}
	sym := c.lookup(id.Name)
	if sym == nil {
		c.p.errorf(c.fc.file.Path, id.Pos(), "undefined: %s", id.Name)
		return nil
	}
	ref := &ast.Ref{Kind: sym.kind, Name: id.Name, Field: sym.field, Func: sym.fn, Type: sym.typD, Use: sym.use, Decl: sym.decl, TypeVal: sym.typ, Path: c.fc.file.Path}
	id.Ref = ref
	if sym.kind == ast.RefPackage {
		c.fc.mark(id.Name)
	}
	if sym.goType != nil {
		c.p.GoTypes[id] = sym.goType
	}
	return sym.typ
}

func (c *checker) resolveSelector(s *ast.SelectorExpr) ast.Type {
	xt := c.resolveExpr(s.X, false)
	if id, ok := s.X.(*ast.Ident); ok && id.Ref != nil && id.Ref.Kind == ast.RefPackage {
		return c.resolvePackageMember(s, id, xt)
	}
	if n, ok := xt.(*ast.NamedType); ok && n.Decl != nil && n.Decl.Kind == ast.TypeEnum {
		if v := findVariant(n.Decl, s.Name); v != nil {
			s.Ref = &ast.Ref{Kind: ast.RefEnumVariant, Name: s.Name, Variant: v, Type: n.Decl}
			et := &ast.NamedType{Span: s.Span, Name: n.Decl.Name, Decl: n.Decl, GoName: n.Decl.GoName, PkgAlias: n.PkgAlias}
			return et
		}
		c.p.errorf(c.fc.file.Path, s.Pos(), "enum %s has no variant %s", n.Name, s.Name)
		return nil
	}
	if n, ok := xt.(*ast.NamedType); ok && n.Decl != nil && n.Decl.Kind == ast.TypeInterface {
		for _, m := range n.Decl.IFace {
			if m.Name != s.Name {
				continue
			}
			if !c.accessible(n.Decl, s.Name) {
				c.p.errorf(c.fc.file.Path, s.Pos(), "method %s is not exported", s.Name)
			}
			ft := ifaceFuncType(m)
			s.Ref = &ast.Ref{Kind: ast.RefFunc, Name: s.Name, Type: n.Decl, TypeVal: ft}
			return ft
		}
		c.p.errorf(c.fc.file.Path, s.Pos(), "interface %s has no method %s", n.Name, s.Name)
		return nil
	}
	if st := c.structDecl(xt); st != nil {
		if fld := fieldByName(st, s.Name); fld != nil {
			if !c.accessible(st, s.Name) {
				c.p.errorf(c.fc.file.Path, s.Pos(), "field %s is not exported", s.Name)
			}
			s.Ref = &ast.Ref{Kind: ast.RefField, Name: s.Name, Field: fld, Type: st, TypeVal: fld.Type}
			return fld.Type
		}
		if m := methodByName(st, s.Name); m != nil {
			if !c.accessible(st, s.Name) {
				c.p.errorf(c.fc.file.Path, s.Pos(), "method %s is not exported", s.Name)
			}
			ft := c.funcType(m)
			s.Ref = &ast.Ref{Kind: ast.RefFunc, Name: s.Name, Func: m, Type: st, TypeVal: ft}
			if c.fn != nil {
				c.p.edges = append(c.p.edges, ctxEdge{from: c.fn.node(), to: m})
			}
			return ft
		}
	}
	gt := c.goTypeOfExpr(s.X)
	if gt != nil {
		obj, _, _ := types.LookupFieldOrMethod(gt, true, nil, s.Name)
		if obj != nil {
			if !obj.Exported() {
				c.p.errorf(c.fc.file.Path, s.Pos(), "%s is not exported", s.Name)
			}
			s.Ref = &ast.Ref{Kind: ast.RefGlobal, Name: s.Name, Go: obj}
			c.p.GoTypes[s] = obj.Type()
			return c.mapGoType(obj.Type(), s.Pos())
		}
	}
	return nil
}

func (c *checker) resolvePackageMember(s *ast.SelectorExpr, id *ast.Ident, _ ast.Type) ast.Type {
	imp := c.fc.imports[id.Name]
	if imp == nil {
		c.p.errorf(c.fc.file.Path, s.Pos(), "unknown package %s", id.Name)
		return nil
	}
	c.fc.mark(id.Name)
	if imp.Local != nil {
		if fn := imp.Local.Funcs[s.Name]; fn != nil {
			if !exported(s.Name) && imp.Local != c.fc.pkg {
				c.p.errorf(c.fc.file.Path, s.Pos(), "function %s is not exported", s.Name)
			}
			ft := c.funcType(fn)
			s.Ref = &ast.Ref{Kind: ast.RefFunc, Name: s.Name, Func: fn, TypeVal: ft}
			if c.fn != nil {
				c.p.edges = append(c.p.edges, ctxEdge{from: c.fn.node(), to: fn})
			}
			return ft
		}
		if d := imp.Local.Types[s.Name]; d != nil {
			if !exported(s.Name) && imp.Local != c.fc.pkg {
				c.p.errorf(c.fc.file.Path, s.Pos(), "type %s is not exported", s.Name)
			}
			t := &ast.NamedType{Span: s.Span, Package: id.Name, Name: s.Name, Decl: d, GoName: d.GoName, PkgAlias: id.Name}
			s.Ref = &ast.Ref{Kind: ast.RefType, Name: s.Name, Type: d, TypeVal: t}
			return t
		}
		if v := imp.Local.Vars[s.Name]; v != nil {
			if !exported(s.Name) && imp.Local != c.fc.pkg {
				c.p.errorf(c.fc.file.Path, s.Pos(), "variable %s is not exported", s.Name)
			}
			t := c.p.global[imp.Local][s.Name]
			s.Ref = &ast.Ref{Kind: ast.RefGlobal, Name: s.Name, Decl: v, TypeVal: t}
			return t
		}
		c.p.errorf(c.fc.file.Path, s.Pos(), "package %s has no %s", id.Name, s.Name)
		return nil
	}
	if imp.Go == nil || imp.Go.Types == nil {
		return nil
	}
	obj := imp.Go.Types.Scope().Lookup(s.Name)
	if obj == nil {
		c.p.errorf(c.fc.file.Path, s.Pos(), "package %s has no %s", id.Name, s.Name)
		return nil
	}
	if !obj.Exported() {
		c.p.errorf(c.fc.file.Path, s.Pos(), "%s is not exported", s.Name)
		return nil
	}
	s.Ref = &ast.Ref{Kind: ast.RefGlobal, Name: s.Name, Go: obj}
	c.p.GoTypes[s] = obj.Type()
	switch obj := obj.(type) {
	case *types.TypeName:
		t := &ast.NamedType{Span: s.Span, Package: id.Name, Name: s.Name, GoName: s.Name, PkgAlias: id.Name}
		s.Ref.Kind = ast.RefType
		s.Ref.TypeVal = t
		c.p.goNamed[t] = obj.Type()
		return t
	case *types.Func:
		if sig, ok := obj.Type().(*types.Signature); ok {
			return c.typeFromGo(sig, s.Pos())
		}
	}
	return c.mapGoType(obj.Type(), s.Pos())
}

func (c *checker) resolveCall(call *ast.CallExpr) ast.Type {
	var info *ast.CallInfo
	var result ast.Type
	var params []ast.Type
	var goCall bool
	var goResult types.Type
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		c.resolveExpr(fun, false)
		if fun.Ref == nil {
			break
		}
		switch fun.Ref.Kind {
		case ast.RefType:
			info = &ast.CallInfo{Kind: ast.CallConversion, Results: 1, ResultTypes: []ast.Type{fun.Ref.TypeVal}}
			params = []ast.Type{fun.Ref.TypeVal}
			result = fun.Ref.TypeVal
		case ast.RefBuiltin:
			info, result = c.builtinInfo(fun.Name)
		case ast.RefFunc:
			info = callInfoOf(fun.Ref.Func)
			params = info.ParamTypes
			result = firstResult(fun.Ref.Func.Results)
			if c.fn != nil {
				c.p.edges = append(c.p.edges, ctxEdge{from: c.fn.node(), to: fun.Ref.Func, call: call})
			}
		default:
			if ft := c.asFuncType(fun.Ref.TypeVal); ft != nil {
				info = funcTypeInfo(ft)
				params = ft.Params
				result = firstResult(ft.Results)
				if ft.NeedsCtx {
					c.needCtx()
					info.NeedsCtx = true
				}
			}
		}
	case *ast.SelectorExpr:
		c.resolveExpr(fun, false)
		if fun.Ref != nil {
			switch goObj := fun.Ref.Go.(type) {
			case *types.TypeName:
				t := fun.Ref.TypeVal
				info = &ast.CallInfo{Kind: ast.CallConversion, Results: 1, ResultTypes: []ast.Type{t}}
				params = []ast.Type{t}
				result = t
			case *types.Func:
				if sig, ok := goObj.Type().(*types.Signature); ok {
					info, result, goResult = c.goCallInfo(sig, call.Pos())
					params = info.ParamTypes
					if goResult != nil {
						c.p.GoTypes[call] = goResult
					}
					goCall = true
				}
			case *types.Var, *types.Const:
			default:
				if fun.Ref.Kind == ast.RefType {
					t := fun.Ref.TypeVal
					info = &ast.CallInfo{Kind: ast.CallConversion, Results: 1, ResultTypes: []ast.Type{t}}
					params = []ast.Type{t}
					result = t
				} else if fun.Ref.Func != nil {
					info = callInfoOf(fun.Ref.Func)
					params = info.ParamTypes
					result = firstResult(fun.Ref.Func.Results)
				} else if ft := c.asFuncType(fun.Ref.TypeVal); ft != nil {
					info = funcTypeInfo(ft)
					params = ft.Params
					result = firstResult(ft.Results)
				}
			}
		}
	default:
		t := c.resolveExpr(call.Fun, false)
		if ft := c.asFuncType(t); ft != nil {
			info = funcTypeInfo(ft)
			params = ft.Params
			result = firstResult(ft.Results)
		}
	}
	for i, arg := range call.Args {
		var expected ast.Type
		if i < len(params) {
			expected = params[i]
		}
		c.applyNil(arg, expected)
		if lit, ok := arg.(*ast.FuncLit); ok {
			ft := c.asFuncType(expected)
			c.resolveFuncLit(lit, ft)
			c.p.litCalls = append(c.p.litCalls, litCall{lit: lit, goCall: goCall, expected: expected})
			continue
		}
		c.resolveExpr(arg, false)
		if ac, ok := arg.(*ast.CallExpr); ok && ac.Info != nil && ac.Info.Results > 1 {
			c.p.errorf(c.fc.file.Path, arg.Pos(), "multi-result call cannot be used as an argument")
		} else {
			c.checkAssign(expected, arg)
		}
	}
	if info == nil {
		info = &ast.CallInfo{Kind: ast.CallUnknown, Results: 1}
	}
	call.Info = info
	if c.fn != nil {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Ref != nil && sel.Ref.Func != nil && goResult == nil {
			c.p.edges = append(c.p.edges, ctxEdge{from: c.fn.node(), to: sel.Ref.Func, call: call})
		}
	}
	_ = goCall
	return result
}

func ifaceFuncType(m *ast.IFaceMethod) *ast.FuncType {
	ft := &ast.FuncType{Span: m.Span, Results: m.Results, Err: m.Err, NeedsCtx: m.NeedsCtx}
	for _, p := range m.Params {
		ft.Params = append(ft.Params, p.Type)
	}
	return ft
}

func firstResult(ts []ast.Type) ast.Type {
	if len(ts) == 1 {
		return ts[0]
	}
	return nil
}

func (c *checker) asFuncType(t ast.Type) *ast.FuncType {
	switch t := t.(type) {
	case *ast.FuncType:
		return t
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return c.asFuncType(t.Decl.Target)
		}
	}
	return nil
}

func callInfoOf(fn *ast.FuncDecl) *ast.CallInfo {
	info := &ast.CallInfo{Kind: ast.CallFunc, HasError: fn.Err, NeedsCtx: fn.NeedsCtx, Fn: fn}
	info.Results = len(fn.Results)
	info.ResultTypes = append(info.ResultTypes, fn.Results...)
	if fn.Err {
		info.ResultTypes = append(info.ResultTypes, &ast.NamedType{Name: "error", GoName: "error"})
	}
	for _, p := range fn.Params {
		info.ParamTypes = append(info.ParamTypes, p.Type)
	}
	return info
}

func funcTypeInfo(ft *ast.FuncType) *ast.CallInfo {
	info := &ast.CallInfo{Kind: ast.CallFunc, HasError: ft.Err, NeedsCtx: ft.NeedsCtx, ParamTypes: ft.Params}
	info.Results = len(ft.Results)
	info.ResultTypes = append(info.ResultTypes, ft.Results...)
	if ft.Err {
		info.ResultTypes = append(info.ResultTypes, &ast.NamedType{Name: "error", GoName: "error"})
	}
	return info
}

func (c *checker) builtinInfo(name string) (*ast.CallInfo, ast.Type) {
	intT := &ast.NamedType{Name: "int", GoName: "int"}
	boolT := &ast.NamedType{Name: "bool", GoName: "bool"}
	switch name {
	case "len", "cap", "copy":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 1, Builtin: name}, intT
	case "append":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 1, Builtin: name}, nil
	case "delete":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 0, Builtin: name}, nil
	case "has":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 1, Builtin: name}, boolT
	case "panic", "assert", "assert_eq":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 0, Builtin: name}, nil
	case "min", "max":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 1, Builtin: name}, nil
	case "runes":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 1, Builtin: name}, &ast.SliceType{Elem: &ast.NamedType{Name: "rune", GoName: "rune"}}
	case "bytes":
		return &ast.CallInfo{Kind: ast.CallBuiltin, Results: 1, Builtin: name}, &ast.SliceType{Elem: &ast.NamedType{Name: "u8", GoName: "uint8"}}
	}
	return &ast.CallInfo{Kind: ast.CallBuiltin}, nil
}

func (c *checker) resolveType(t ast.Type) ast.Type {
	return c.p.resolveType(c.fc, t, c.tparams)
}

func (c *checker) resolveOr(or *ast.OrExpr, top bool) ast.Type {
	if !top {
		c.p.errorf(c.fc.file.Path, or.Pos(), "or must be the whole initializer, right side, statement or return value")
	}
	xt := c.resolveExpr(or.X, false)
	call, ok := or.X.(*ast.CallExpr)
	if !ok {
		c.p.errorf(c.fc.file.Path, or.Pos(), "or expects a call")
		return xt
	}
	if call.Info == nil || !call.Info.HasError {
		c.p.errorf(c.fc.file.Path, or.Pos(), "call does not return an error")
		return xt
	}
	if call.Info.Results > 1 {
		c.p.errorf(c.fc.file.Path, or.Pos(), "or supports at most one non-error result")
	}
	switch or.Mode {
	case ast.OrReturn:
		if c.fn == nil {
			c.p.errorf(c.fc.file.Path, or.Pos(), "or return outside a function")
		} else if !c.fn.err {
			c.p.errorf(c.fc.file.Path, or.Pos(), "or return in a function that does not return an error")
		}
	case ast.OrPanic:
		c.resolveExpr(or.PanicArg, false)
	case ast.OrHandle:
		c.push()
		c.declare(&symbol{name: or.Name, kind: ast.RefLocal, typ: &ast.NamedType{Name: "error", GoName: "error"}, decl: or})
		c.checkBlock(or.Block)
		c.pop()
	}
	return xt
}

func (c *checker) applyNil(e ast.Expr, want ast.Type) {
	if n, ok := e.(*ast.NilLit); ok && n.Type == nil && want != nil {
		n.Type = want
	}
}

func (c *checker) nilable(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.PointerType, *ast.SliceType, *ast.MapType, *ast.FuncType:
		return true
	case *ast.NamedType:
		if t.Package == "" && (t.Name == "any" || t.Name == "error") {
			return true
		}
		if t.Decl != nil {
			switch t.Decl.Kind {
			case ast.TypeInterface, ast.TypeEnum:
				return true
			case ast.TypeAlias:
				return c.nilable(t.Decl.Target)
			default:
				return false
			}
		}
	}
	if gt := c.goTypeOf(t); gt != nil {
		switch gt.Underlying().(type) {
		case *types.Pointer, *types.Slice, *types.Map, *types.Signature, *types.Interface:
			return true
		}
	}
	return false
}

func typeName(t ast.Type) string {
	if n, ok := t.(*ast.NamedType); ok {
		return n.Name
	}
	return "this type"
}

func (c *checker) resolveComposite(e *ast.CompositeLit) ast.Type {
	t := c.p.resolveType(c.fc, e.Type, c.tparams)
	e.Type = t
	if n, ok := t.(*ast.NamedType); ok && n.Variant != nil {
		v := n.Variant
		e.Variant = v
		c.resolveFields(e, v.Fields, v.Enum)
		return &ast.NamedType{Span: n.Span, Name: v.Enum.Name, Decl: v.Enum, GoName: v.Enum.GoName, PkgAlias: n.PkgAlias}
	}
	if st := c.structDecl(t); st != nil {
		c.resolveFields(e, st.Fields, st)
		return t
	}
	switch ty := t.(type) {
	case *ast.MapType:
		for _, en := range e.Entries {
			if en.KeyExpr == nil {
				c.p.errorf(c.fc.file.Path, en.Pos(), "map literal entry needs key = value")
				continue
			}
			c.applyNil(en.KeyExpr, ty.Key)
			c.applyNil(en.Value, ty.Value)
			c.resolveExpr(en.KeyExpr, false)
			c.resolveExpr(en.Value, false)
			c.checkAssign(ty.Value, en.Value)
		}
	case *ast.SliceType, *ast.ArrayType:
		var elem ast.Type
		switch ty := ty.(type) {
		case *ast.SliceType:
			elem = ty.Elem
		case *ast.ArrayType:
			elem = ty.Elem
		}
		for _, en := range e.Entries {
			c.applyNil(en.Value, elem)
			c.resolveExpr(en.Value, false)
			c.checkAssign(elem, en.Value)
		}
	case *ast.GenericType:
		for _, en := range e.Entries {
			c.resolveExpr(en.Value, false)
		}
	case *ast.NamedType:
		if ty.Decl == nil {
			for _, en := range e.Entries {
				c.resolveExpr(en.Value, false)
			}
		}
	}
	return t
}

func (c *checker) resolveFields(e *ast.CompositeLit, fields []*ast.Field, owner *ast.TypeDecl) {
	seen := make(map[string]bool)
	for _, en := range e.Entries {
		if en.Key == "" {
			c.p.errorf(c.fc.file.Path, en.Pos(), "struct literal entry needs field = value")
			continue
		}
		f := fieldByName2(fields, en.Key)
		if f == nil {
			c.p.errorf(c.fc.file.Path, en.Pos(), "unknown field %s", en.Key)
			continue
		}
		if seen[en.Key] {
			c.p.errorf(c.fc.file.Path, en.Pos(), "field %s repeated", en.Key)
		}
		seen[en.Key] = true
		if owner != nil && !c.accessible(owner, f.Name) {
			c.p.errorf(c.fc.file.Path, en.Pos(), "field %s is not exported", f.Name)
		}
		en.Field = f
		c.applyNil(en.Value, f.Type)
		c.resolveExpr(en.Value, false)
		c.checkAssign(f.Type, en.Value)
	}
}

func fieldByName2(fields []*ast.Field, name string) *ast.Field {
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (c *checker) resolveFuncLit(lit *ast.FuncLit, expected *ast.FuncType) ast.Type {
	var params []ast.Type
	if expected != nil {
		params = expected.Params
	}
	for i, p := range lit.Params {
		var t ast.Type
		if p.Type != nil {
			t = c.p.resolveType(c.fc, p.Type, c.tparams)
			p.Type = t
		} else if i < len(params) {
			t = params[i]
			p.Type = t
		} else {
			c.p.errorf(c.fc.file.Path, p.Pos(), "cannot infer parameter type")
		}
		lit.ParamTypes = append(lit.ParamTypes, t)
	}
	var results []ast.Type
	if len(lit.Results) > 0 {
		results = c.p.normalizeResults(c.fc, lit.Results, c.tparams)
		lit.Results = results
	} else if expected != nil {
		results = expected.Results
		lit.Results = results
		lit.Err = expected.Err
	}
	fi := &funcInfo{lit: lit, results: results, err: lit.Err, file: c.fc}
	c.p.funcs[lit] = fi
	if c.fn != nil {
		c.p.edges = append(c.p.edges, ctxEdge{from: c.fn.node(), to: lit})
	}
	savedFn := c.fn
	savedInPar, savedLoop, savedMatch := c.inPar, c.loopDepth, c.matchDepth
	c.fn = fi
	c.inPar = false
	c.loopDepth = 0
	c.matchDepth = 0
	c.push()
	for i, p := range lit.Params {
		c.declare(&symbol{name: p.Name, kind: ast.RefParam, typ: p.Type, goType: c.goTypeOf(p.Type), decl: p})
		if i < len(lit.ParamTypes) && p.Type == nil {
			p.Type = lit.ParamTypes[i]
		}
	}
	if lit.Arrow != nil {
		if len(results) == 1 {
			c.applyNil(lit.Arrow, results[0])
		}
		at := c.resolveExpr(lit.Arrow, false)
		if len(results) == 0 {
			results = []ast.Type{at}
			lit.Results = results
			fi.results = results
		} else if len(results) > 1 {
			c.p.errorf(c.fc.file.Path, lit.Pos(), "arrow function literal cannot return several values")
		}
	} else {
		c.checkBlock(lit.Body)
	}
	c.pop()
	c.fn = savedFn
	c.inPar = savedInPar
	c.loopDepth = savedLoop
	c.matchDepth = savedMatch
	ft := &ast.FuncType{Span: lit.Span, Params: lit.ParamTypes, Results: results, Err: lit.Err}
	return ft
}

func (c *checker) resolveMatch(m *ast.MatchExpr) ast.Type {
	subj := c.resolveExpr(m.Subject, false)
	m.SubjectType = subj
	open := c.isOpenType(subj)
	m.TypeSwitch = open
	var enum *ast.TypeDecl
	if !open {
		if n, ok := subj.(*ast.NamedType); ok && n.Decl != nil && n.Decl.Kind == ast.TypeEnum {
			enum = n.Decl
		}
	}
	covered := make(map[string]bool)
	hasWildcard := false
	defaults := 0
	var out ast.Type
	for _, cs := range m.Cases {
		pat := cs.Pat
		if cs.Default || pat == nil || pat.Wildcard {
			hasWildcard = true
			defaults++
			if defaults > 1 {
				c.p.errorf(c.fc.file.Path, cs.Pos(), "multiple else cases")
			}
		} else if open {
			c.resolveTypePattern(pat, cs)
		} else if pat.TypePat != nil || pat.Bind != "" {
			c.p.errorf(c.fc.file.Path, pat.Pos(), "type patterns are only allowed on any and interfaces")
		} else if enum != nil {
			v := variantByPattern(enum, pat)
			if v == nil {
				c.p.errorf(c.fc.file.Path, pat.Pos(), "enum %s has no variant %s", enum.Name, pat.Name)
			} else {
				if covered[v.Name] {
					c.p.errorf(c.fc.file.Path, pat.Pos(), "variant %s repeated", v.Name)
				}
				covered[v.Name] = true
				pat.Variant = v
				c.bindPatternFields(cs, pat, v.Fields)
			}
		} else if pat.Name != "" && pat.Qualifier != "" {
			sel := &ast.SelectorExpr{Span: pat.Span, X: &ast.Ident{Span: pat.Span, Name: pat.Qualifier, Ref: &ast.Ref{Kind: ast.RefPackage, Name: pat.Qualifier}}, Name: pat.Name}
			c.resolveSelector(sel)
			pat.Literal = sel
		} else if pat.Name != "" {
			if v := c.uniqueVariant(pat.Name); v != nil {
				pat.Variant = v
				if enum == nil {
					enum = v.Enum
					m.SubjectType = &ast.NamedType{Name: v.Enum.Name, Decl: v.Enum, GoName: v.Enum.GoName}
				}
				covered[v.Name] = true
				c.bindPatternFields(cs, pat, v.Fields)
			} else if d := c.fc.pkg.Vars[pat.Name]; d != nil && d.Const {
				pat.Literal = &ast.Ident{Span: pat.Span, Name: pat.Name, Ref: &ast.Ref{Kind: ast.RefConst, Name: pat.Name, Decl: d}}
			} else {
				c.p.errorf(c.fc.file.Path, pat.Pos(), "unknown pattern %s", pat.Name)
			}
		} else if pat.Literal != nil {
			c.resolveExpr(pat.Literal, false)
		}
		c.matchDepth++
		c.push()
		if pat.Variant != nil {
			for i, name := range pat.Fields {
				if i < len(pat.FieldDecls) && pat.FieldDecls[i] != nil {
					c.declare(&symbol{name: name, kind: ast.RefLocal, typ: pat.FieldDecls[i].Type, decl: cs})
				} else {
					c.declare(&symbol{name: name, kind: ast.RefLocal, decl: cs})
				}
			}
		}
		if open && pat != nil && pat.Bind != "" && pat.Bind != "_" {
			c.declare(&symbol{name: pat.Bind, kind: ast.RefLocal, typ: pat.TypePat, goType: c.goTypeOf(pat.TypePat), decl: cs})
		}
		if cs.Expr != nil {
			t := c.resolveExpr(cs.Expr, false)
			if out == nil {
				out = t
			}
		} else if cs.Block != nil {
			c.checkBlock(cs.Block)
		} else {
			for _, s := range cs.Body {
				c.checkStmt(s)
			}
		}
		c.pop()
		c.matchDepth--
	}
	if enum != nil && !hasWildcard {
		for _, v := range enum.Variants {
			if !covered[v.Name] {
				c.p.errorf(c.fc.file.Path, m.Pos(), "match is not exhaustive: missing %s", v.Name)
			}
		}
	}
	if open && !hasWildcard {
		c.p.errorf(c.fc.file.Path, m.Pos(), "match on any or interface requires else")
	}
	if m.IsExpr {
		for _, cs := range m.Cases {
			if cs.Expr == nil {
				c.p.errorf(c.fc.file.Path, cs.Pos(), "expected an expression case")
				break
			}
		}
		m.ResultType = out
	}
	return out
}

func (c *checker) resolveTypePattern(pat *ast.Pattern, cs *ast.Case) {
	if pat.Literal != nil {
		c.p.errorf(c.fc.file.Path, pat.Pos(), "match on any requires type patterns")
		return
	}
	if len(pat.Fields) > 0 {
		c.p.errorf(c.fc.file.Path, pat.Pos(), "type pattern cannot bind fields")
	}
	if pat.TypePat == nil {
		pat.TypePat = &ast.NamedType{Span: pat.Span, Package: pat.Qualifier, Name: pat.Name}
	}
	pat.TypePat = c.p.resolveType(c.fc, pat.TypePat, c.tparams)
}

func (c *checker) isOpenType(t ast.Type) bool {
	if n, ok := t.(*ast.NamedType); ok {
		if n.Package == "" && (n.Name == "any" || n.Name == "error") {
			return true
		}
		if n.Decl != nil && n.Decl.Kind == ast.TypeInterface {
			return true
		}
	}
	if gt := c.goTypeOf(t); gt != nil {
		if _, ok := gt.Underlying().(*types.Interface); ok {
			return true
		}
	}
	return false
}

func (c *checker) bindPatternFields(cs *ast.Case, pat *ast.Pattern, fields []*ast.Field) {
	seen := make(map[string]bool)
	for _, name := range pat.Fields {
		if seen[name] {
			c.p.errorf(c.fc.file.Path, pat.Pos(), "field %s repeated", name)
		}
		seen[name] = true
		f := fieldByName2(fields, name)
		if f == nil {
			c.p.errorf(c.fc.file.Path, pat.Pos(), "unknown field %s", name)
			continue
		}
		pat.FieldDecls = append(pat.FieldDecls, f)
	}
}

func (c *checker) uniqueVariant(name string) *ast.Variant {
	var found *ast.Variant
	for _, pkg := range c.p.Packages {
		for _, d := range pkg.Types {
			if d.Kind != ast.TypeEnum {
				continue
			}
			if v := findVariant(d, name); v != nil {
				if found != nil && found != v {
					return nil
				}
				found = v
			}
		}
	}
	return found
}

func (c *checker) structDecl(t ast.Type) *ast.TypeDecl {
	switch t := t.(type) {
	case *ast.NamedType:
		if t.Decl == nil {
			return nil
		}
		if t.Decl.Kind == ast.TypeStruct {
			return t.Decl
		}
		if t.Decl.Kind == ast.TypeAlias {
			return c.structDecl(t.Decl.Target)
		}
	case *ast.PointerType:
		return c.structDecl(t.Elem)
	}
	return nil
}

func fieldByName(d *ast.TypeDecl, name string) *ast.Field {
	return fieldByName2(d.Fields, name)
}

func methodByName(d *ast.TypeDecl, name string) *ast.FuncDecl {
	for _, m := range d.Methods {
		if m.Name == name {
			return m
		}
	}
	return nil
}

func (c *checker) accessible(d *ast.TypeDecl, name string) bool {
	if exported(name) {
		return true
	}
	return c.fc.pkg.Types[d.Name] == d
}

func elemType(t ast.Type) ast.Type {
	switch t := t.(type) {
	case *ast.PointerType:
		return t.Elem
	case *ast.SliceType:
		return t.Elem
	case *ast.ArrayType:
		return t.Elem
	case *ast.MapType:
		return t.Value
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return elemType(t.Decl.Target)
		}
	}
	return nil
}

func (c *checker) goCallInfo(sig *types.Signature, pos token.Pos) (*ast.CallInfo, ast.Type, types.Type) {
	info := &ast.CallInfo{Kind: ast.CallFunc}
	n := sig.Params().Len()
	start := 0
	if n > 0 && isContextType(sig.Params().At(0).Type()) {
		info.NeedsCtx = true
		start = 1
	}
	for i := start; i < n; i++ {
		pt := sig.Params().At(i).Type()
		if sig.Variadic() && i == n-1 {
			if sl, ok := pt.(*types.Slice); ok {
				pt = sl.Elem()
			}
		}
		info.ParamTypes = append(info.ParamTypes, c.mapGoType(pt, pos))
	}
	results := sig.Results()
	if results.Len() > 0 && isErrorType(results.At(results.Len()-1).Type()) {
		info.HasError = true
		info.Results = results.Len() - 1
	} else {
		info.Results = results.Len()
	}
	var result ast.Type
	var goResult types.Type
	if info.Results == 1 && results.Len() > 0 {
		goResult = results.At(0).Type()
		result = c.mapGoType(goResult, pos)
	}
	if info.HasError && results.Len() == 1 && info.Results == 0 {
		goResult = results.At(0).Type()
		result = c.mapGoType(goResult, pos)
	}
	for i := 0; i < results.Len(); i++ {
		if info.HasError && i == results.Len()-1 {
			info.ResultTypes = append(info.ResultTypes, &ast.NamedType{Name: "error", GoName: "error"})
		} else {
			info.ResultTypes = append(info.ResultTypes, c.mapGoType(results.At(i).Type(), pos))
		}
	}
	if info.NeedsCtx {
		c.needCtx()
	}
	return info, result, goResult
}

func isErrorType(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

func isContextType(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := n.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == "context" && obj.Name() == "Context"
}

func (c *checker) mapGoType(t types.Type, pos token.Pos) ast.Type {
	if t == nil {
		return nil
	}
	if sig, ok := t.(*types.Signature); ok {
		ft := &ast.FuncType{Span: ast.Span{Start: pos, End: pos}}
		start := 0
		if sig.Params().Len() > 0 && isContextType(sig.Params().At(0).Type()) {
			ft.NeedsCtx = true
			start = 1
		}
		for i := start; i < sig.Params().Len(); i++ {
			pt := sig.Params().At(i).Type()
			if sig.Variadic() && i == sig.Params().Len()-1 {
				if sl, ok := pt.(*types.Slice); ok {
					pt = sl.Elem()
				}
			}
			ft.Params = append(ft.Params, c.mapGoType(pt, pos))
		}
		res := sig.Results()
		last := res.Len()
		if last > 0 && isErrorType(res.At(last-1).Type()) {
			ft.Err = true
			last--
		}
		for i := 0; i < last; i++ {
			ft.Results = append(ft.Results, c.mapGoType(res.At(i).Type(), pos))
		}
		return ft
	}
	q := func(pkg *types.Package) string {
		if pkg == nil {
			return ""
		}
		for alias, imp := range c.fc.imports {
			if imp.Go != nil && imp.Go.Path == pkg.Path() {
				return alias
			}
		}
		return pkg.Name()
	}
	gt := &ast.GoType{Span: ast.Span{Start: pos, End: pos}, Text: types.TypeString(t, q)}
	c.p.goOf[gt] = t
	return gt
}

func (c *checker) typeFromGo(sig *types.Signature, pos token.Pos) ast.Type {
	return c.mapGoType(sig, pos)
}

func (c *checker) goTypeOf(t ast.Type) types.Type {
	return c.p.goTypeOf(t)
}

func (p *Program) goTypeOf(t ast.Type) types.Type {
	if t == nil {
		return nil
	}
	if gt := p.goOf[t]; gt != nil {
		return gt
	}
	switch t := t.(type) {
	case *ast.GoType:
		return p.goOf[t]
	case *ast.NamedType:
		if goT := p.goNamed[t]; goT != nil {
			return goT
		}
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return p.goTypeOf(t.Decl.Target)
		}
	case *ast.PointerType:
		if e := p.goTypeOf(t.Elem); e != nil {
			return types.NewPointer(e)
		}
	case *ast.SliceType:
		if e := p.goTypeOf(t.Elem); e != nil {
			return types.NewSlice(e)
		}
	case *ast.MapType:
		k := p.goTypeOf(t.Key)
		v := p.goTypeOf(t.Value)
		if k != nil && v != nil {
			return types.NewMap(k, v)
		}
	}
	return nil
}

func (c *checker) goTypeOfExpr(e ast.Expr) types.Type {
	if t := c.p.GoTypes[e]; t != nil {
		return t
	}
	if t := c.p.ExprTypes[e]; t != nil {
		return c.goTypeOf(t)
	}
	return nil
}

func (c *checker) exprType(e ast.Expr) ast.Type {
	if e == nil {
		return nil
	}
	if t := c.p.ExprTypes[e]; t != nil {
		return t
	}
	switch e := e.(type) {
	case *ast.Ident:
		if e.Ref != nil {
			return e.Ref.TypeVal
		}
	case *ast.BasicLit:
		return litType(e)
	case *ast.CompositeLit:
		return e.Type
	case *ast.MatchExpr:
		return e.ResultType
	case *ast.FuncLit:
		return &ast.FuncType{Params: e.ParamTypes, Results: e.Results, Err: e.Err}
	case *ast.OrExpr:
		return c.exprType(e.X)
	}
	if t := c.p.GoTypes[e]; t != nil && c.fc != nil {
		return c.mapGoType(t, e.Pos())
	}
	return nil
}

func (c *checker) checkParAssignment(s *ast.AssignStmt) {
	_ = s
}

func (c *checker) typeNameOf(t ast.Type) string {
	switch t := t.(type) {
	case *ast.NamedType:
		if t.PkgAlias != "" {
			return t.PkgAlias + "." + t.GoName
		}
		if t.GoName != "" {
			return t.GoName
		}
		return t.Name
	case *ast.GoType:
		return t.Text
	case *ast.PointerType:
		return "*" + c.typeNameOf(t.Elem)
	case *ast.SliceType:
		return "[]" + c.typeNameOf(t.Elem)
	}
	return strings.TrimSpace("")
}
