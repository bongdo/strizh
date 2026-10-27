package sema

import (
	"go/types"
	"strconv"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/token"
)

type symbol struct {
	name   string
	kind   ast.RefKind
	typ    ast.Type
	goType types.Type
	decl   ast.Node
	field  *ast.Field
	fn     *ast.FuncDecl
	typD   *ast.TypeDecl
	use    *ast.Use
}

type scope struct {
	parent *scope
	names  map[string]*symbol
}

type parFrame struct {
	iter      map[string]bool
	plain     bool
	loopDecls map[ast.Node]bool
}

type checker struct {
	p          *Program
	fc         *fileCtx
	fn         *funcInfo
	scopes     []*scope
	loopDepth  int
	matchDepth int
	parFrames  []parFrame
	inPar      bool
	tparams    map[string]bool
}

func (c *checker) push() {
	c.scopes = append(c.scopes, &scope{names: make(map[string]*symbol), parent: c.top()})
}

func (c *checker) pop() {
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *checker) top() *scope {
	if len(c.scopes) == 0 {
		return nil
	}
	return c.scopes[len(c.scopes)-1]
}

func (c *checker) declare(sym *symbol) {
	if sym.name == "_" {
		return
	}
	if sc := c.top(); sc != nil {
		sc.names[sym.name] = sym
	}
}

func (c *checker) lookupLocal(name string) *symbol {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if s := c.scopes[i].names[name]; s != nil {
			return s
		}
	}
	return nil
}

func (c *checker) lookup(name string) *symbol {
	if s := c.lookupLocal(name); s != nil {
		return s
	}
	if d := c.fc.pkg.Types[name]; d != nil {
		return &symbol{name: name, kind: ast.RefType, typ: &ast.NamedType{Span: d.Span, Name: name, Decl: d, GoName: d.GoName}, decl: d, typD: d}
	}
	if d := c.fc.pkg.Funcs[name]; d != nil {
		return &symbol{name: name, kind: ast.RefFunc, typ: c.funcType(d), decl: d, fn: d}
	}
	if d := c.fc.pkg.Vars[name]; d != nil {
		kind := ast.RefGlobal
		if d.Const {
			kind = ast.RefConst
		}
		return &symbol{name: name, kind: kind, typ: c.p.global[c.fc.pkg][name], decl: d}
	}
	if imp := c.fc.imports[name]; imp != nil {
		return &symbol{name: name, kind: ast.RefPackage, use: c.fc.uses[name], typ: nil}
	}
	if IsBuiltinType(name) {
		return &symbol{name: name, kind: ast.RefType, typ: &ast.NamedType{Name: name, GoName: builtinTypes[name]}}
	}
	if isBuiltinFunc(name) {
		return &symbol{name: name, kind: ast.RefBuiltin, typ: nil}
	}
	return nil
}

func (c *checker) funcType(fn *ast.FuncDecl) ast.Type {
	ft := &ast.FuncType{Span: fn.Span, Results: fn.Results, Err: fn.Err}
	for _, p := range fn.Params {
		ft.Params = append(ft.Params, p.Type)
	}
	return ft
}

func isBuiltinFunc(name string) bool {
	switch name {
	case "len", "cap", "append", "copy", "delete", "has", "panic", "assert", "assert_eq", "min", "max", "runes", "bytes":
		return true
	}
	return false
}

func (p *Program) checkFuncs() {
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			fc := p.fileCtx(pkg, f)
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					c := &checker{p: p, fc: fc, fn: p.funcs[d]}
					c.checkFunction(d, nil)
				case *ast.TypeDecl:
					for _, m := range d.Methods {
						c := &checker{p: p, fc: fc, fn: p.funcs[m]}
						c.checkFunction(m, nil)
					}
				}
			}
		}
	}
}

func (c *checker) checkFunction(fn *ast.FuncDecl, outer *checker) {
	if outer != nil {
		c.scopes = outer.scopes
	}
	c.push()
	c.tparams = mergeParams(c.tparams, typeParamSet(fn.TypeParams))
	if fn.Owner != nil {
		c.tparams = mergeParams(c.tparams, typeParamSet(fn.Owner.TypeParams))
		for _, tp := range fn.Owner.TypeParams {
			c.declare(&symbol{name: tp.Name, kind: ast.RefType, typ: &ast.NamedType{Span: tp.Span, Name: tp.Name}})
		}
	}
	for _, tp := range fn.TypeParams {
		c.declare(&symbol{name: tp.Name, kind: ast.RefType, typ: &ast.NamedType{Span: tp.Span, Name: tp.Name}})
	}
	if fn.Owner != nil {
		c.push()
		c.declare(&symbol{name: "self", kind: ast.RefSelf, typ: &ast.PointerType{Span: fn.Owner.Span, Elem: &ast.NamedType{Span: fn.Owner.Span, Name: fn.Owner.Name, Decl: fn.Owner, GoName: fn.Owner.GoName}}, decl: fn})
		for _, fld := range fn.Owner.Fields {
			c.declare(&symbol{name: fld.Name, kind: ast.RefField, typ: fld.Type, decl: fld, field: fld})
		}
	}
	for _, param := range fn.Params {
		c.declare(&symbol{name: param.Name, kind: ast.RefParam, typ: param.Type, goType: c.goTypeOf(param.Type), decl: param})
	}
	c.checkBlock(fn.Body)
	if outer != nil {
		c.scopes = nil
	}
}

func (c *checker) checkBlock(b *ast.Block) {
	if b == nil {
		return
	}
	c.push()
	for _, s := range b.Stmts {
		c.checkStmt(s)
	}
	c.pop()
}

func (p *Program) checkTests() {
	for _, pkg := range p.Packages {
		seen := make(map[string]int)
		for _, f := range pkg.Files {
			fc := p.fileCtx(pkg, f)
			for _, d := range f.Decls {
				t, ok := d.(*ast.TestDecl)
				if !ok {
					continue
				}
				if t.Name == "" {
					p.errorf(f.Path, t.Pos(), "test name is empty")
				}
				id := testGoID(t.Name)
				if n := seen[id]; n > 0 {
					seen[id] = n + 1
					id = id + "_" + strconv.Itoa(n)
				} else {
					seen[id] = 1
				}
				t.GoID = id
				fi := &funcInfo{results: nil, err: true, file: fc, test: t}
				c := &checker{p: p, fc: fc, fn: fi}
				c.checkBlock(t.Body)
			}
		}
	}
}

func testGoID(name string) string {
	var b []rune
	up := true
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			if up && r >= 'a' && r <= 'z' {
				r = r - 'a' + 'A'
			}
			b = append(b, r)
			up = false
		} else {
			up = true
		}
	}
	if len(b) == 0 {
		return "Unnamed"
	}
	if b[0] >= '0' && b[0] <= '9' {
		b = append([]rune("T"), b...)
	}
	return string(b)
}

func (c *checker) checkStmt(s ast.Stmt) {
	if s == nil {
		return
	}
	switch s := s.(type) {
	case *ast.Block:
		c.checkBlock(s)
	case *ast.EmptyStmt:
	case *ast.DeclStmt:
		c.checkVarDecl(s.Decl)
	case *ast.AssignStmt:
		var lhsTypes []ast.Type
		for _, e := range s.Lhs {
			lhsTypes = append(lhsTypes, c.resolveExpr(e, false))
		}
		for i, e := range s.Rhs {
			if len(s.Rhs) == len(lhsTypes) {
				c.applyNil(e, lhsTypes[i])
			}
			c.resolveExpr(e, true)
			if len(s.Rhs) == len(lhsTypes) {
				c.checkAssign(lhsTypes[i], e)
			}
			if len(s.Lhs) == 1 && len(s.Rhs) == 1 {
				if call, ok := e.(*ast.CallExpr); ok && call.Info != nil && call.Info.Results > 1 {
					c.p.errorf(c.fc.file.Path, call.Pos(), "multi-result call needs matching names")
				}
			}
		}
		if len(s.Rhs) > 1 {
			for _, e := range s.Rhs {
				if hasOr(e) {
					c.p.errorf(c.fc.file.Path, e.Pos(), "or needs a single right side")
				}
			}
		}
	case *ast.ExprStmt:
		c.resolveExpr(s.X, true)
	case *ast.IfStmt:
		c.resolveExpr(s.Cond, false)
		c.checkErrGuard(s)
		c.checkBlock(s.Then)
		if s.Else != nil {
			c.push()
			c.checkStmt(s.Else)
			c.pop()
		}
	case *ast.ForStmt:
		c.checkFor(s)
	case *ast.ReturnStmt:
		if c.fn == nil {
			c.p.errorf(c.fc.file.Path, s.Pos(), "return outside a function")
		}
		if c.fn != nil {
			for i, e := range s.Results {
				var want ast.Type
				if i < len(c.fn.results) {
					want = c.fn.results[i]
				} else if c.fn.err && i == len(c.fn.results) {
					want = &ast.NamedType{Name: "error", GoName: "error"}
				}
				c.applyNil(e, want)
			}
		}
		for i, e := range s.Results {
			c.resolveExpr(e, true)
			var want ast.Type
			if c.fn != nil {
				if i < len(c.fn.results) {
					want = c.fn.results[i]
				} else if c.fn.err && i == len(c.fn.results) {
					want = &ast.NamedType{Name: "error", GoName: "error"}
				}
			}
			c.checkAssign(want, e)
		}
		if len(s.Results) > 1 {
			for _, e := range s.Results {
				if hasOr(e) {
					c.p.errorf(c.fc.file.Path, e.Pos(), "or needs a single return value")
				}
			}
		}
		if c.fn != nil {
			n := len(c.fn.results)
			if len(s.Results) != n && !(c.fn.err && len(s.Results) == n+1) {
				if n == 0 && !c.fn.err && len(s.Results) > 0 {
					c.p.errorf(c.fc.file.Path, s.Pos(), "return value in a void function")
				} else if len(s.Results) == 1 {
					if call, ok := s.Results[0].(*ast.CallExpr); ok && call.Info != nil && call.Info.Results == n && (n > 0 || call.Info.HasError) {
					} else {
						c.p.errorf(c.fc.file.Path, s.Pos(), "wrong number of return values: got %d, want %d", len(s.Results), n)
					}
				} else {
					c.p.errorf(c.fc.file.Path, s.Pos(), "wrong number of return values: got %d, want %d", len(s.Results), n)
				}
			}
		}
		if c.inPar {
			c.p.errorf(c.fc.file.Path, s.Pos(), "return cannot leave a par block; use or return")
		}
	case *ast.BreakStmt:
		if c.loopDepth == 0 {
			c.p.errorf(c.fc.file.Path, s.Pos(), "break outside a loop")
		}
		if c.matchDepth > 0 {
			c.p.errorf(c.fc.file.Path, s.Pos(), "break inside a match case is not supported")
		}
		if c.inPar {
			c.p.errorf(c.fc.file.Path, s.Pos(), "break cannot leave a par block")
		}
	case *ast.ContinueStmt:
		if c.loopDepth == 0 {
			c.p.errorf(c.fc.file.Path, s.Pos(), "continue outside a loop")
		}
		if c.inPar {
			c.p.errorf(c.fc.file.Path, s.Pos(), "continue cannot leave a par block")
		}
	case *ast.DeferStmt:
		if s.Call == nil {
			c.p.errorf(c.fc.file.Path, s.Pos(), "defer expects a call")
		} else {
			c.resolveExpr(s.Call, false)
		}
	case *ast.ParStmt:
		c.checkPar(s.Body, nil)
	case *ast.WithinStmt:
		c.resolveExpr(s.Duration, false)
		c.needCtx()
		c.checkBlock(s.Body)
	}
}

func (c *checker) checkErrGuard(s *ast.IfStmt) {
	err := errGuard(s.Cond)
	if err == nil || s.Then == nil || len(s.Then.Stmts) != 1 {
		return
	}
	ret, ok := s.Then.Stmts[0].(*ast.ReturnStmt)
	if !ok || !c.isErrorAstType(c.exprType(err)) {
		return
	}
	for _, r := range ret.Results {
		if id, ok := unparen(r).(*ast.Ident); ok && sameIdent(err, id) {
			c.p.errorf(c.fc.file.Path, s.Pos(), "use or return instead of if err != nil { return err }")
			return
		}
	}
}

func errGuard(cond ast.Expr) *ast.Ident {
	bin, ok := unparen(cond).(*ast.BinaryExpr)
	if !ok || bin.Op != token.Ne {
		return nil
	}
	x, y := unparen(bin.X), unparen(bin.Y)
	if _, ok := x.(*ast.NilLit); ok {
		x, y = y, x
	}
	if _, ok := y.(*ast.NilLit); !ok {
		return nil
	}
	id, _ := x.(*ast.Ident)
	return id
}

func sameIdent(a, b *ast.Ident) bool {
	if a.Name != b.Name {
		return false
	}
	return a.Ref == nil || b.Ref == nil || a.Ref.Decl == b.Ref.Decl
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func (c *checker) isErrorAstType(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return c.isErrorAstType(t.Decl.Target)
		}
		return t.Package == "" && t.Name == "error"
	case *ast.GoType:
		return isErrorType(c.p.goOf[t])
	}
	return false
}

func (c *checker) checkFor(f *ast.ForStmt) {
	c.push()
	if f.Range != nil {
		t := c.resolveExpr(f.Range.X, false)
		iter := make(map[string]bool)
		for _, n := range f.Range.Names {
			iter[n] = true
		}
		elem, key, isMap, known := c.rangeTypes(t)
		f.Range.Known = known
		f.Range.IsMap = isMap
		f.Range.KeyType = key
		f.Range.ElemType = elem
		switch len(f.Range.Names) {
		case 1:
			if isMap {
				c.declare(&symbol{name: f.Range.Names[0], kind: ast.RefLocal, typ: key, decl: f.Range})
			} else {
				c.declare(&symbol{name: f.Range.Names[0], kind: ast.RefLocal, typ: elem, decl: f.Range})
			}
		case 2:
			c.declare(&symbol{name: f.Range.Names[0], kind: ast.RefLocal, typ: key, decl: f.Range})
			c.declare(&symbol{name: f.Range.Names[1], kind: ast.RefLocal, typ: elem, decl: f.Range})
		}
		if f.Limit != nil {
			c.resolveExpr(f.Limit, false)
		}
		if f.Par {
			c.needCtx()
			frame := parFrame{iter: iter, loopDecls: map[ast.Node]bool{f.Range: true}}
			c.parFrames = append(c.parFrames, frame)
			oldInPar := c.inPar
			c.inPar = true
			c.loopDepth++
			c.checkBlock(f.Body)
			c.loopDepth--
			c.inPar = oldInPar
			c.parFrames = c.parFrames[:len(c.parFrames)-1]
			c.checkParWrites(f.Body, frame)
		} else {
			c.loopDepth++
			c.checkBlock(f.Body)
			c.loopDepth--
		}
	} else {
		if f.Init != nil {
			c.checkStmt(f.Init)
		}
		if f.Cond != nil {
			c.resolveExpr(f.Cond, false)
		}
		if f.Post != nil {
			if stmtHasOr(f.Post) {
				c.p.errorf(c.fc.file.Path, f.Post.Pos(), "or is not supported in a for post statement")
			}
			c.checkStmt(f.Post)
		}
		if f.Par && f.Init != nil {
			switch init := f.Init.(type) {
			case *ast.DeclStmt:
			case *ast.AssignStmt:
				if init.Op != token.Assign {
					c.p.errorf(c.fc.file.Path, init.Pos(), "par for init must assign simple names")
				}
				for _, l := range init.Lhs {
					if _, ok := l.(*ast.Ident); !ok {
						c.p.errorf(c.fc.file.Path, l.Pos(), "par for init must assign simple names")
					}
				}
			default:
				c.p.errorf(c.fc.file.Path, f.Init.Pos(), "par for init must declare or assign simple names")
			}
		}
		if f.Par {
			iter := make(map[string]bool)
			loopDecls := make(map[ast.Node]bool)
			switch init := f.Init.(type) {
			case *ast.DeclStmt:
				if init.Decl != nil {
					for _, n := range init.Decl.Names {
						iter[n] = true
					}
					loopDecls[init.Decl] = true
				}
			case *ast.AssignStmt:
				for _, l := range init.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						iter[id.Name] = true
					}
				}
			}
			c.needCtx()
			frame := parFrame{iter: iter, loopDecls: loopDecls}
			c.parFrames = append(c.parFrames, frame)
			oldInPar := c.inPar
			c.inPar = true
			c.loopDepth++
			c.checkBlock(f.Body)
			c.loopDepth--
			c.inPar = oldInPar
			c.parFrames = c.parFrames[:len(c.parFrames)-1]
			c.checkParWrites(f.Body, frame)
		} else {
			c.loopDepth++
			c.checkBlock(f.Body)
			c.loopDepth--
		}
	}
	c.pop()
}

func (c *checker) rangeTypes(t ast.Type) (elem, key ast.Type, isMap, known bool) {
	switch t := t.(type) {
	case *ast.SliceType:
		return t.Elem, &ast.NamedType{Name: "int", GoName: "int"}, false, true
	case *ast.ArrayType:
		return t.Elem, &ast.NamedType{Name: "int", GoName: "int"}, false, true
	case *ast.MapType:
		return t.Value, t.Key, true, true
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return c.rangeTypes(t.Decl.Target)
		}
		if t.Decl == nil && t.Package == "" && t.Name == "str" {
			return &ast.NamedType{Name: "rune", GoName: "rune"}, &ast.NamedType{Name: "int", GoName: "int"}, false, true
		}
		if gt := c.goTypeOf(t); gt != nil {
			if m, ok := gt.Underlying().(*types.Map); ok {
				return c.mapGoType(m.Elem(), t.Pos()), c.mapGoType(m.Key(), t.Pos()), true, true
			}
			if s, ok := gt.Underlying().(*types.Slice); ok {
				return c.mapGoType(s.Elem(), t.Pos()), &ast.NamedType{Name: "int", GoName: "int"}, false, true
			}
		}
	case *ast.GoType:
		if gt := c.p.goOf[t]; gt != nil {
			switch u := gt.Underlying().(type) {
			case *types.Map:
				return c.mapGoType(u.Elem(), t.Pos()), c.mapGoType(u.Key(), t.Pos()), true, true
			case *types.Slice:
				return c.mapGoType(u.Elem(), t.Pos()), &ast.NamedType{Name: "int", GoName: "int"}, false, true
			case *types.Array:
				return c.mapGoType(u.Elem(), t.Pos()), &ast.NamedType{Name: "int", GoName: "int"}, false, true
			case *types.Basic:
				if u.Kind() == types.String {
					return &ast.NamedType{Name: "rune", GoName: "rune"}, &ast.NamedType{Name: "int", GoName: "int"}, false, true
				}
			}
		}
	}
	return nil, nil, false, false
}

func (c *checker) checkVarDecl(v *ast.VarDecl) {
	if v == nil {
		return
	}
	var types []ast.Type
	for _, val := range v.Values {
		types = append(types, c.resolveExpr(val, true))
	}
	if len(v.Values) == 1 {
		if call, ok := v.Values[0].(*ast.CallExpr); ok && call.Info != nil && len(call.Info.ResultTypes) > 0 {
			switch {
			case len(call.Info.ResultTypes) == len(v.Names):
				types = call.Info.ResultTypes
			case len(v.Names) == 1 && call.Info.Results > 1:
				c.p.errorf(c.fc.file.Path, call.Pos(), "multi-result call needs matching names")
			case len(v.Names) > 1:
				c.p.errorf(c.fc.file.Path, call.Pos(), "wrong number of results: got %d, want %d", len(call.Info.ResultTypes), len(v.Names))
			}
		}
	}
	if len(v.Values) > 1 {
		for _, val := range v.Values {
			if hasOr(val) {
				c.p.errorf(c.fc.file.Path, val.Pos(), "or needs a single initializer")
			}
		}
	}
	for i, name := range v.Names {
		if name == "_" {
			continue
		}
		var t ast.Type
		if len(types) == len(v.Names) {
			t = types[i]
		}
		c.declare(&symbol{name: name, kind: ast.RefLocal, typ: t, goType: c.goTypeOf(t), decl: v})
	}
}

func (c *checker) checkPar(body *ast.Block, _ map[string]bool) {
	c.needCtx()
	frame := parFrame{plain: true}
	c.parFrames = append(c.parFrames, frame)
	oldInPar := c.inPar
	c.inPar = true
	c.push()
	for _, s := range body.Stmts {
		c.push()
		c.checkStmt(s)
		c.pop()
	}
	c.pop()
	c.inPar = oldInPar
	c.parFrames = c.parFrames[:len(c.parFrames)-1]
	c.checkParWrites(body, frame)
}

func (c *checker) needCtx() {
	if c.fn != nil {
		c.p.needsCtx[c.fn.node()] = true
		if c.fn.decl != nil {
			c.p.needsCtx[c.fn.decl] = true
		}
		if c.fn.lit != nil {
			c.p.needsCtx[c.fn.lit] = true
		}
	}
}

func (f *funcInfo) node() ast.Node {
	if f.decl != nil {
		return f.decl
	}
	if f.lit != nil {
		return f.lit
	}
	if f.test != nil {
		return f.test
	}
	return nil
}

func hasOr(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if _, ok := n.(*ast.OrExpr); ok {
			found = true
			return false
		}
		return true
	})
	return found
}

func stmtHasOr(s ast.Stmt) bool {
	found := false
	ast.Inspect(s, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if _, ok := n.(*ast.OrExpr); ok {
			found = true
			return false
		}
		return true
	})
	return found
}

func (c *checker) collectInner(b *ast.Block) map[ast.Node]bool {
	inner := make(map[ast.Node]bool)
	ast.Inspect(b, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.VarDecl:
			inner[n] = true
		case *ast.RangeClause:
			inner[n] = true
		}
		return true
	})
	return inner
}

type parWrite struct {
	id     *ast.Ident
	pos    token.Pos
	viaPtr bool
	viaMap bool
}

type parStmtEffects struct {
	writes []parWrite
	reads  map[string]bool
}

type parWalker struct {
	c     *checker
	frame parFrame
	inner map[ast.Node]bool
	alias map[string][]*ast.Ident
	eff   *parStmtEffects
}

func (c *checker) checkParWrites(b *ast.Block, frame parFrame) {
	alias := c.collectParAliases(b)
	if c.fn != nil {
		var body *ast.Block
		if c.fn.decl != nil {
			body = c.fn.decl.Body
		} else if c.fn.lit != nil {
			body = c.fn.lit.Body
		} else if c.fn.test != nil {
			body = c.fn.test.Body
		}
		if body != nil {
			for name, roots := range c.collectParAliases(body) {
				alias[name] = append(alias[name], roots...)
			}
		}
	}
	var stmts []parStmtEffects
	if frame.plain {
		stmts = make([]parStmtEffects, len(b.Stmts))
		for i, s := range b.Stmts {
			inner := c.collectInner(&ast.Block{Stmts: []ast.Stmt{s}})
			w := &parWalker{
				c:     c,
				frame: frame,
				inner: inner,
				alias: alias,
				eff:   &parStmtEffects{reads: make(map[string]bool)},
			}
			w.stmt(s)
			stmts[i] = *w.eff
		}
	} else {
		inner := c.collectInner(b)
		for d := range frame.loopDecls {
			inner[d] = true
		}
		w := &parWalker{
			c:     c,
			frame: frame,
			inner: inner,
			alias: alias,
			eff:   &parStmtEffects{reads: make(map[string]bool)},
		}
		for _, s := range b.Stmts {
			w.stmt(s)
		}
		stmts = []parStmtEffects{*w.eff}
	}
	writers := make(map[string]int)
	readers := make(map[string]map[int]bool)
	for i := range stmts {
		seen := make(map[string]bool)
		for _, w := range stmts[i].writes {
			if seen[w.id.Name] {
				continue
			}
			seen[w.id.Name] = true
			writers[w.id.Name]++
		}
		for r := range stmts[i].reads {
			if readers[r] == nil {
				readers[r] = make(map[int]bool)
			}
			readers[r][i] = true
		}
	}
	for i := range stmts {
		seen := make(map[string]bool)
		for _, w := range stmts[i].writes {
			if seen[w.id.Name] {
				continue
			}
			seen[w.id.Name] = true
			c.checkParWrite(w, i, frame, writers, readers)
		}
	}
}

func (c *checker) checkParWrite(w parWrite, stmt int, frame parFrame, writers map[string]int, readers map[string]map[int]bool) {
	if w.id.Ref == nil {
		return
	}
	switch w.id.Ref.Kind {
	case ast.RefSelf, ast.RefField:
		c.p.errorf(c.fc.file.Path, w.pos, "writing to self inside par")
	case ast.RefGlobal:
		c.p.errorf(c.fc.file.Path, w.pos, "writing to global %s inside par", w.id.Name)
	case ast.RefParam, ast.RefLocal:
		t := c.exprType(w.id)
		if c.isMapType(t) {
			c.p.errorf(c.fc.file.Path, w.pos, "writing to map %s inside par", w.id.Name)
			return
		}
		if c.isPointerType(t) {
			c.p.errorf(c.fc.file.Path, w.pos, "writing to %s captured from outside par", w.id.Name)
			return
		}
		if w.viaMap {
			c.p.errorf(c.fc.file.Path, w.pos, "writing to map %s inside par", w.id.Name)
			return
		}
		if w.viaPtr {
			c.p.errorf(c.fc.file.Path, w.pos, "writing to %s captured from outside par", w.id.Name)
			return
		}
		if frame.plain && writers[w.id.Name] == 1 {
			if !parReadByOther(w.id.Name, stmt, readers) {
				return
			}
			c.p.errorf(c.fc.file.Path, w.pos, "read by another par statement: %s", w.id.Name)
			return
		}
		c.p.errorf(c.fc.file.Path, w.pos, "writing to %s captured from outside par", w.id.Name)
	}
}

func (c *checker) isMapType(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.MapType:
		return true
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return c.isMapType(t.Decl.Target)
		}
	case *ast.GoType:
		gt := c.p.goOf[t]
		if gt != nil {
			_, ok := gt.Underlying().(*types.Map)
			return ok
		}
	}
	return false
}

func (c *checker) isInterfaceType(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.InterfaceType:
		return true
	case *ast.NamedType:
		if t.Package == "" && (t.Name == "any" || t.Name == "error") {
			return true
		}
		if t.Decl != nil {
			switch t.Decl.Kind {
			case ast.TypeInterface:
				return true
			case ast.TypeAlias:
				return c.isInterfaceType(t.Decl.Target)
			}
		}
	case *ast.GoType:
		gt := c.p.goOf[t]
		if gt != nil {
			_, ok := gt.Underlying().(*types.Interface)
			return ok
		}
	}
	if gt := c.goTypeOf(t); gt != nil {
		_, ok := gt.Underlying().(*types.Interface)
		return ok
	}
	return false
}

func (c *checker) writeMap(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.IndexExpr:
		if c.isMapType(c.exprType(x.X)) {
			return true
		}
		return c.writeMap(x.X)
	case *ast.SelectorExpr:
		if c.isMapType(c.exprType(x)) {
			return true
		}
		return c.writeMap(x.X)
	case *ast.UnaryExpr:
		return c.writeMap(x.X)
	case *ast.ParenExpr:
		return c.writeMap(x.X)
	}
	return false
}

func (c *checker) writeThroughPointer(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		if c.isPointerType(c.exprType(x.X)) {
			return true
		}
		return c.writeThroughPointer(x.X)
	case *ast.IndexExpr:
		if c.isPointerType(c.exprType(x.X)) {
			return true
		}
		return c.writeThroughPointer(x.X)
	case *ast.UnaryExpr:
		if x.Op == token.Star && c.isPointerType(c.exprType(x.X)) {
			return true
		}
		return c.writeThroughPointer(x.X)
	case *ast.ParenExpr:
		return c.writeThroughPointer(x.X)
	}
	return false
}

func (c *checker) isPointerType(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.PointerType:
		return true
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return c.isPointerType(t.Decl.Target)
		}
	case *ast.GoType:
		gt := c.p.goOf[t]
		if gt != nil {
			_, ok := gt.Underlying().(*types.Pointer)
			return ok
		}
	}
	return false
}

func parReadByOther(name string, stmt int, readers map[string]map[int]bool) bool {
	for i := range readers[name] {
		if i != stmt {
			return true
		}
	}
	return false
}

func (w *parWalker) stmt(s ast.Stmt) {
	if s == nil {
		return
	}
	switch s := s.(type) {
	case *ast.Block:
		for _, x := range s.Stmts {
			w.stmt(x)
		}
	case *ast.DeclStmt:
		if s.Decl != nil {
			for _, v := range s.Decl.Values {
				w.expr(v)
			}
		}
	case *ast.AssignStmt:
		for _, lhs := range s.Lhs {
			w.expr(lhs)
			if !w.frame.plain && w.c.disjoint(lhs, w.frame.iter) && !w.globalRoot(lhs) {
				continue
			}
			w.write(lhs)
		}
		for _, r := range s.Rhs {
			w.expr(r)
		}
	case *ast.ExprStmt:
		w.expr(s.X)
	case *ast.IfStmt:
		w.expr(s.Cond)
		w.stmt(s.Then)
		w.stmt(s.Else)
	case *ast.ForStmt:
		if s.Range != nil {
			w.expr(s.Range.X)
		} else {
			w.stmt(s.Init)
			w.expr(s.Cond)
			w.stmt(s.Post)
		}
		w.stmt(s.Body)
	case *ast.ParStmt:
		w.stmt(s.Body)
	case *ast.WithinStmt:
		w.expr(s.Duration)
		w.stmt(s.Body)
	case *ast.ReturnStmt:
		for _, e := range s.Results {
			w.expr(e)
		}
	case *ast.DeferStmt:
		w.expr(s.Call)
	}
}

func (w *parWalker) expr(e ast.Expr) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.Ident:
		w.read(e)
	case *ast.FuncLit:
		return
	case *ast.CallExpr:
		w.call(e)
	case *ast.BinaryExpr:
		w.expr(e.X)
		w.expr(e.Y)
	case *ast.UnaryExpr:
		w.expr(e.X)
	case *ast.ParenExpr:
		w.expr(e.X)
	case *ast.IndexExpr:
		w.expr(e.X)
		w.expr(e.Index)
	case *ast.SliceExpr:
		w.expr(e.X)
		w.expr(e.Low)
		w.expr(e.High)
	case *ast.SelectorExpr:
		w.expr(e.X)
	case *ast.CompositeLit:
		for _, en := range e.Entries {
			w.expr(en.KeyExpr)
			w.expr(en.Value)
		}
	case *ast.MatchExpr:
		w.expr(e.Subject)
		for _, cs := range e.Cases {
			if cs.Block != nil {
				w.stmt(cs.Block)
			}
			if cs.Expr != nil {
				w.expr(cs.Expr)
			}
			for _, s := range cs.Body {
				w.stmt(s)
			}
		}
	case *ast.OrExpr:
		w.expr(e.X)
		if e.Block != nil {
			w.stmt(e.Block)
		}
		if e.PanicArg != nil {
			w.expr(e.PanicArg)
		}
	}
}

func (w *parWalker) call(e *ast.CallExpr) {
	if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
		if !parPackageRef(sel.X) {
			w.method(sel)
		} else {
			w.expr(sel.X)
		}
	} else {
		w.expr(e.Fun)
	}
	for _, a := range e.Args {
		w.expr(a)
	}
	if id, ok := e.Fun.(*ast.Ident); ok {
		w.mutatingBuiltin(e, id)
	}
}

func parPackageRef(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Ref != nil && id.Ref.Kind == ast.RefPackage
}

func (w *parWalker) method(sel *ast.SelectorExpr) {
	w.expr(sel.X)
	if w.pureMethod(sel) || w.slotMethod(sel.X) {
		return
	}
	w.writeMethod(sel.X)
}

func (w *parWalker) writeMethod(e ast.Expr) {
	root := rootIdent(e)
	if root == nil {
		return
	}
	viaPtr := w.c.isPointerType(w.c.exprType(e)) || w.c.isInterfaceType(w.c.exprType(e))
	for _, t := range w.targets(root) {
		if w.isInner(t) {
			continue
		}
		w.eff.writes = append(w.eff.writes, parWrite{id: t, pos: e.Pos(), viaPtr: viaPtr})
	}
}

func (w *parWalker) isInner(id *ast.Ident) bool {
	return id != nil && id.Ref != nil && id.Ref.Decl != nil && w.inner[id.Ref.Decl]
}

func (w *parWalker) pureMethod(sel *ast.SelectorExpr) bool {
	return sel.Ref != nil && sel.Ref.Func != nil && pureParMethod(sel.Ref.Func)
}

func (w *parWalker) slotMethod(recv ast.Expr) bool {
	if w.frame.plain || !w.c.disjoint(recv, w.frame.iter) || w.globalRoot(recv) {
		return false
	}
	return w.c.structValue(w.c.exprType(recv))
}

func (w *parWalker) globalRoot(e ast.Expr) bool {
	root := rootIdent(e)
	return root != nil && root.Ref != nil && root.Ref.Kind == ast.RefGlobal
}

func (w *parWalker) mutatingBuiltin(e *ast.CallExpr, id *ast.Ident) {
	switch id.Name {
	case "delete", "copy":
	default:
		return
	}
	if len(e.Args) == 0 {
		return
	}
	if !w.frame.plain && w.c.disjoint(e.Args[0], w.frame.iter) && !w.globalRoot(e.Args[0]) {
		return
	}
	w.write(e.Args[0])
}

func (w *parWalker) read(id *ast.Ident) {
	if id.Name == "_" || id.Ref == nil || !parVarRef(id.Ref.Kind) {
		return
	}
	for _, t := range w.targets(id) {
		if w.isInner(t) || t.Ref == nil || !parVarRef(t.Ref.Kind) {
			continue
		}
		w.eff.reads[t.Name] = true
	}
}

func (w *parWalker) write(e ast.Expr) {
	root := rootIdent(e)
	if root == nil {
		return
	}
	viaPtr := w.c.writeThroughPointer(e)
	viaMap := w.c.writeMap(e)
	for _, t := range w.targets(root) {
		if w.isInner(t) {
			continue
		}
		w.eff.writes = append(w.eff.writes, parWrite{id: t, pos: e.Pos(), viaPtr: viaPtr, viaMap: viaMap})
	}
}

func (w *parWalker) targets(root *ast.Ident) []*ast.Ident {
	seen := make(map[string]bool)
	var out []*ast.Ident
	var walk func(*ast.Ident)
	walk = func(id *ast.Ident) {
		if id == nil || seen[id.Name] {
			return
		}
		seen[id.Name] = true
		if next := w.alias[id.Name]; len(next) > 0 {
			for _, t := range next {
				walk(t)
			}
			return
		}
		out = append(out, id)
	}
	walk(root)
	return out
}

func parVarRef(k ast.RefKind) bool {
	switch k {
	case ast.RefLocal, ast.RefParam, ast.RefGlobal, ast.RefSelf, ast.RefField:
		return true
	}
	return false
}

func (c *checker) collectParAliases(b *ast.Block) map[string][]*ast.Ident {
	edges := make(map[string][]*ast.Ident)
	add := func(name string, v ast.Expr) {
		if name == "_" || !c.refLike(c.exprType(v)) {
			return
		}
		for {
			if p, ok := v.(*ast.ParenExpr); ok {
				v = p.X
				continue
			}
			if u, ok := v.(*ast.UnaryExpr); ok && u.Op == token.Amp {
				v = u.X
				continue
			}
			break
		}
		root := rootIdent(v)
		if root == nil || root.Ref == nil || !parVarRef(root.Ref.Kind) {
			return
		}
		edges[name] = append(edges[name], root)
	}
	var walkExpr func(ast.Expr)
	var walkStmt func(ast.Stmt)
	walkExpr = func(e ast.Expr) {
		switch e := e.(type) {
		case *ast.MatchExpr:
			for _, cs := range e.Cases {
				if cs.Block != nil {
					walkStmt(cs.Block)
				}
				if cs.Expr != nil {
					walkExpr(cs.Expr)
				}
				for _, s := range cs.Body {
					walkStmt(s)
				}
			}
		case *ast.OrExpr:
			walkExpr(e.X)
			if e.Block != nil {
				walkStmt(e.Block)
			}
			if e.PanicArg != nil {
				walkExpr(e.PanicArg)
			}
		}
	}
	walkStmt = func(s ast.Stmt) {
		switch s := s.(type) {
		case *ast.Block:
			for _, x := range s.Stmts {
				walkStmt(x)
			}
		case *ast.DeclStmt:
			if s.Decl != nil {
				for i, name := range s.Decl.Names {
					if i < len(s.Decl.Values) {
						add(name, s.Decl.Values[i])
					}
				}
			}
		case *ast.AssignStmt:
			if s.Op != token.Assign || len(s.Lhs) != len(s.Rhs) {
				return
			}
			for i, lhs := range s.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					add(id.Name, s.Rhs[i])
				}
			}
		case *ast.ExprStmt:
			walkExpr(s.X)
		case *ast.IfStmt:
			walkStmt(s.Then)
			if s.Else != nil {
				walkStmt(s.Else)
			}
		case *ast.ForStmt:
			if s.Range == nil {
				walkStmt(s.Init)
				walkStmt(s.Post)
			}
			walkStmt(s.Body)
		case *ast.ParStmt:
			walkStmt(s.Body)
		case *ast.WithinStmt:
			walkStmt(s.Body)
		}
	}
	for _, s := range b.Stmts {
		walkStmt(s)
	}
	return edges
}

func (c *checker) refLike(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.PointerType, *ast.SliceType, *ast.MapType, *ast.InterfaceType:
		return true
	case *ast.NamedType:
		if t.Decl != nil {
			switch t.Decl.Kind {
			case ast.TypeAlias:
				return c.refLike(t.Decl.Target)
			case ast.TypeInterface:
				return true
			}
		}
		return t.Package == "" && t.Name == "any"
	case *ast.GoType:
		gt := c.p.goOf[t]
		if gt != nil {
			switch gt.Underlying().(type) {
			case *types.Pointer, *types.Slice, *types.Map, *types.Interface:
				return true
			}
		}
	}
	return false
}

func pureParMethod(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	pure := true
	var walkStmt func(ast.Stmt)
	var walkExpr func(ast.Expr)
	walkStmt = func(s ast.Stmt) {
		if !pure || s == nil {
			return
		}
		switch s := s.(type) {
		case *ast.Block:
			for _, x := range s.Stmts {
				walkStmt(x)
			}
		case *ast.DeclStmt:
			if s.Decl != nil {
				for _, v := range s.Decl.Values {
					walkExpr(v)
				}
			}
		case *ast.AssignStmt:
			pure = false
		case *ast.ExprStmt:
			walkExpr(s.X)
		case *ast.IfStmt:
			walkExpr(s.Cond)
			walkStmt(s.Then)
			walkStmt(s.Else)
		case *ast.ForStmt:
			if s.Range != nil {
				walkExpr(s.Range.X)
			} else {
				walkStmt(s.Init)
				walkExpr(s.Cond)
				walkStmt(s.Post)
			}
			walkStmt(s.Body)
		case *ast.ReturnStmt:
			for _, e := range s.Results {
				walkExpr(e)
			}
		case *ast.DeferStmt, *ast.ParStmt, *ast.WithinStmt:
			pure = false
		}
	}
	walkExpr = func(e ast.Expr) {
		if !pure || e == nil {
			return
		}
		switch e := e.(type) {
		case *ast.FuncLit:
			pure = false
		case *ast.CallExpr:
			if !pureParCall(e) {
				pure = false
				return
			}
			for _, a := range e.Args {
				walkExpr(a)
			}
		case *ast.BinaryExpr:
			walkExpr(e.X)
			walkExpr(e.Y)
		case *ast.UnaryExpr:
			walkExpr(e.X)
		case *ast.ParenExpr:
			walkExpr(e.X)
		case *ast.IndexExpr:
			walkExpr(e.X)
			walkExpr(e.Index)
		case *ast.SliceExpr:
			walkExpr(e.X)
			walkExpr(e.Low)
			walkExpr(e.High)
		case *ast.SelectorExpr:
			walkExpr(e.X)
		case *ast.CompositeLit:
			for _, en := range e.Entries {
				walkExpr(en.KeyExpr)
				walkExpr(en.Value)
			}
		case *ast.MatchExpr:
			walkExpr(e.Subject)
			for _, cs := range e.Cases {
				if cs.Block != nil {
					walkStmt(cs.Block)
				}
				if cs.Expr != nil {
					walkExpr(cs.Expr)
				}
				for _, s := range cs.Body {
					walkStmt(s)
				}
			}
		case *ast.OrExpr:
			walkExpr(e.X)
			if e.Block != nil {
				walkStmt(e.Block)
			}
			if e.PanicArg != nil {
				walkExpr(e.PanicArg)
			}
		}
	}
	walkStmt(fn.Body)
	return pure
}

func pureParCall(e *ast.CallExpr) bool {
	id, ok := e.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	switch id.Name {
	case "len", "cap", "min", "max", "has", "assert", "assert_eq", "panic", "str", "runes", "bytes":
		return true
	}
	return IsBuiltinType(id.Name) || (id.Ref != nil && id.Ref.Kind == ast.RefType)
}

func rootIdent(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.SliceExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.UnaryExpr:
			if x.Op == token.Star {
				e = x.X
				continue
			}
			return nil
		default:
			return nil
		}
	}
}

func (c *checker) disjoint(e ast.Expr, iter map[string]bool) bool {
	if !c.sliceIndexChain(e) {
		return false
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && iter[id.Name] {
			found = true
			return false
		}
		return true
	})
	return found
}

func (c *checker) sliceIndexChain(e ast.Expr) bool {
	saw := false
	for {
		switch x := e.(type) {
		case *ast.IndexExpr:
			saw = true
			if !c.sliceOrArray(c.exprType(x.X)) {
				return false
			}
			e = x.X
		case *ast.SelectorExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.UnaryExpr:
			if x.Op == token.Star {
				e = x.X
				continue
			}
			return false
		case *ast.Ident:
			return saw
		default:
			return false
		}
	}
}

func (c *checker) sliceOrArray(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.SliceType, *ast.ArrayType:
		return true
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return c.sliceOrArray(t.Decl.Target)
		}
	case *ast.GoType:
		gt := c.p.goOf[t]
		if gt != nil {
			switch gt.Underlying().(type) {
			case *types.Slice, *types.Array:
				return true
			}
		}
	}
	return false
}

func (c *checker) structValue(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.StructType:
		return true
	case *ast.NamedType:
		if t.Decl != nil {
			switch t.Decl.Kind {
			case ast.TypeAlias:
				return c.structValue(t.Decl.Target)
			case ast.TypeStruct:
				return true
			}
		}
	case *ast.GoType:
		gt := c.p.goOf[t]
		if gt != nil {
			_, ok := gt.Underlying().(*types.Struct)
			return ok
		}
	}
	return false
}
