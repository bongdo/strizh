package gen

import (
	"fmt"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
)

func (g *generator) decl(w *writer, d ast.Decl) {
	switch d := d.(type) {
	case *ast.TypeDecl:
		g.typeDecl(w, d)
	case *ast.FuncDecl:
		g.funcDecl(w, d, nil)
	case *ast.VarDecl:
		g.packageVar(w, d)
	}
}

func (g *generator) funcDecl(w *writer, fd *ast.FuncDecl, recv *ast.TypeDecl) {
	name := fd.GoName
	if name == "" {
		name = fd.Name
	}
	main := recv == nil && fd.Name == "main" && w.f.pkg.IsMain
	if main {
		name = "_s_main"
	}
	if fd.NeedsCtx && !main {
		g.ctxFuncDecl(w, fd, recv, name)
		return
	}
	w.directive(fd)
	params := g.paramsStr(w.f, fd.Params, nil, fd.NeedsCtx)
	res := g.resultStr(w.f, fd.Results, fd.Err)
	sig := "func "
	if recv != nil {
		selfName := methodSelfName(fd, recv)
		w.self = selfName
		sig += "(" + selfName + " *" + typeNameOf(recv) + g.typeParamsUse(recv.TypeParams) + ") "
	}
	sig += name
	if recv == nil {
		sig += g.typeParamsDecl(fd.TypeParams)
	}
	sig += "(" + params + ")" + res
	w.line(sig + " {")
	w.indent++
	g.funcBody(w, fd.Body, fd.Results, fd.Err, fd.NeedsCtx)
	w.indent--
	w.line("}")
	w.line("")
	if main {
		g.mainWrapper(w, fd)
	}
}

func (g *generator) ctxFuncDecl(w *writer, fd *ast.FuncDecl, recv *ast.TypeDecl, name string) {
	params := safeParams(fd.Params)
	selfName := ""
	if recv != nil {
		selfName = methodSelfName(fd, recv)
	}
	res := g.resultStr(w.f, fd.Results, fd.Err)
	ctxName := ctxFuncName(fd)

	w.directive(fd)
	sig := "func "
	if recv != nil {
		sig += "(" + selfName + " *" + typeNameOf(recv) + g.typeParamsUse(recv.TypeParams) + ") "
	}
	sig += name
	if recv == nil {
		sig += g.typeParamsDecl(fd.TypeParams)
	}
	sig += "(" + g.paramsStr(w.f, params, nil, false) + ")" + res
	w.line(sig + " {")
	w.indent++
	w.f.needPkg("context")
	callArgs := make([]string, 0, len(params)+2)
	if recv != nil {
		callArgs = append(callArgs, selfName)
	}
	callArgs = append(callArgs, "_s_context.Background()")
	for _, p := range params {
		callArgs = append(callArgs, p.Name)
	}
	call := ctxName
	if recv == nil && len(fd.TypeParams) > 0 {
		call += g.typeParamsUse(fd.TypeParams)
	} else if recv != nil && len(recv.TypeParams) > 0 {
		call += g.typeParamsUse(recv.TypeParams)
	}
	call += "(" + strings.Join(callArgs, ", ") + ")"
	if len(fd.Results) == 0 && !fd.Err {
		w.line(call)
	} else {
		w.line("return " + call)
	}
	w.indent--
	w.line("}")
	w.line("")

	w.directive(fd)
	sig = "func " + ctxName
	if recv == nil {
		sig += g.typeParamsDecl(fd.TypeParams)
	} else {
		sig += g.typeParamsDecl(recv.TypeParams)
	}
	sig += "("
	var parts []string
	if recv != nil {
		parts = append(parts, selfName+" *"+typeNameOf(recv)+g.typeParamsUse(recv.TypeParams))
	}
	parts = append(parts, "_s_ctx _s_context.Context")
	if len(params) > 0 {
		parts = append(parts, g.paramsStr(w.f, params, nil, false))
	}
	sig += strings.Join(parts, ", ") + ")" + res
	w.line(sig + " {")
	w.indent++
	if recv != nil {
		w.self = selfName
	}
	g.funcBody(w, fd.Body, fd.Results, fd.Err, true)
	w.indent--
	w.line("}")
	w.line("")
}

func methodSelfName(fd *ast.FuncDecl, recv *ast.TypeDecl) string {
	for _, p := range fd.Params {
		if p.Name == "self" {
			return "_s_self"
		}
	}
	for _, tp := range recv.TypeParams {
		if tp.Name == "self" {
			return "_s_self"
		}
	}
	if hasLocalSelf(fd.Body) {
		return "_s_self"
	}
	return "self"
}

func typeNameOf(recv *ast.TypeDecl) string {
	if recv.GoName != "" {
		return recv.GoName
	}
	return recv.Name
}

func safeParams(params []*ast.Param) []*ast.Param {
	out := make([]*ast.Param, 0, len(params))
	for i, p := range params {
		name := p.Name
		if name == "" || name == "_" {
			name = fmt.Sprintf("_s_a%d", i)
		}
		out = append(out, &ast.Param{Span: p.Span, Name: name, Type: p.Type})
	}
	return out
}

func ctxFuncName(fd *ast.FuncDecl) string {
	name := fd.GoName
	if name == "" {
		name = fd.Name
	}
	if fd.Owner != nil {
		return "_s_" + typeNameOf(fd.Owner) + "_" + name + "_ctx"
	}
	return "_s_" + name + "_ctx"
}

func (g *generator) funcBody(w *writer, body *ast.Block, results []ast.Type, err bool, needsCtx bool) {
	w.results = results
	w.err = err
	if needsCtx {
		w.ctx = "_s_ctx"
		w.f.needPkg("context")
	}
	if bodyHasDefer(body) {
		w.deferStack = true
		w.f.needPkg("rt")
		w.line("var _s_d _s_rt.DeferStack")
		w.line("defer _s_d.RunTo(0)")
	}
	var stmts []ast.Stmt
	if body != nil {
		stmts = body.Stmts
	}
	w.line("{")
	w.indent++
	w.bodyContents(body, hasDirectDefer(stmts))
	w.indent--
	w.line("}")
	if err && len(results) == 0 {
		w.line("return nil")
	}
	if len(results) > 0 {
		w.line(`panic("unreachable")`)
	}
}

func (g *generator) mainWrapper(w *writer, fd *ast.FuncDecl) {
	w.directive(fd)
	w.line("func main() {")
	w.indent++
	args := ""
	if fd.NeedsCtx {
		w.line("_s_ctx := _s_context.Background()")
		args = "_s_ctx"
		w.f.needPkg("context")
	}
	call := "_s_main(" + args + ")"
	if fd.Err {
		w.f.needPkg("fmt")
		w.f.needPkg("os")
		w.linef("if _s_err := %s; _s_err != nil {", call)
		w.indent++
		w.line("_s_fmt.Fprintln(_s_os.Stderr, _s_err)")
		w.line("_s_os.Exit(1)")
		w.indent--
		w.line("}")
	} else {
		w.line(call)
	}
	w.indent--
	w.line("}")
	w.line("")
}

func (g *generator) typeDecl(w *writer, d *ast.TypeDecl) {
	name := d.GoName
	if name == "" {
		name = d.Name
	}
	switch d.Kind {
	case ast.TypeAlias:
		w.directive(d)
		w.line("type " + name + g.typeParamsDecl(d.TypeParams) + " = " + g.typeStr(w.f, d.Target))
		w.line("")
	case ast.TypeStruct:
		g.structDecl(w, d, name)
	case ast.TypeEnum:
		g.enumDecl(w, d, name)
	case ast.TypeInterface:
		w.directive(d)
		if len(d.IFace) == 0 {
			w.line("type " + name + g.typeParamsDecl(d.TypeParams) + " interface{}")
			w.line("")
			return
		}
		w.line("type " + name + g.typeParamsDecl(d.TypeParams) + " interface {")
		w.indent++
		for _, m := range d.IFace {
			w.line(g.ifaceMethodStr(w.f, m))
		}
		w.indent--
		w.line("}")
		w.line("")
	}
}

func (g *generator) structDecl(w *writer, d *ast.TypeDecl, name string) {
	w.directive(d)
	if len(d.Fields) == 0 {
		w.line("type " + name + g.typeParamsDecl(d.TypeParams) + " struct{}")
	} else {
		w.line("type " + name + g.typeParamsDecl(d.TypeParams) + " struct {")
		w.indent++
		for _, fld := range d.Fields {
			w.line(fld.Name + " " + g.typeStr(w.f, fld.Type))
		}
		w.indent--
		w.line("}")
	}
	w.line("")
	for _, m := range d.Methods {
		mw := newWriter(w.f, "_s_context.Background()")
		g.funcDecl(mw, m, d)
		w.buf.WriteString(mw.buf.String())
	}
}

func (g *generator) enumDecl(w *writer, d *ast.TypeDecl, name string) {
	marker := "_s_is" + name
	w.directive(d)
	w.line("type " + name + " interface{ " + marker + "() }")
	w.line("")
	for _, v := range d.Variants {
		vt := g.variantName(v)
		w.directive(v)
		if len(v.Fields) == 0 {
			w.line("type " + vt + " struct{}")
		} else {
			w.line("type " + vt + " struct {")
			w.indent++
			for _, fld := range v.Fields {
				w.line(fld.Name + " " + g.typeStr(w.f, fld.Type))
			}
			w.indent--
			w.line("}")
		}
		w.directive(v)
		w.line("func (*" + vt + ") " + marker + "() {}")
		w.line("")
	}
}

func (g *generator) packageVar(w *writer, d *ast.VarDecl) {
	for _, v := range d.Values {
		if _, ok := v.(*ast.OrExpr); ok {
			g.errAt(v, "or is not supported in a package-level declaration")
			return
		}
	}
	if len(d.Values) == 0 {
		return
	}
	values := make([]string, 0, len(d.Values))
	for _, v := range d.Values {
		values = append(values, w.expr(v))
	}
	prefix := "var"
	if d.Const {
		prefix = "const"
	}
	w.directive(d)
	w.line(prefix + " " + strings.Join(d.Names, ", ") + " = " + strings.Join(values, ", "))
	w.line("")
}

func (g *generator) testDecl(w *writer, t *ast.TestDecl) {
	id := t.GoID
	if id == "" {
		id = "Unnamed"
	}
	wrapper := "Test" + id
	helper := "_s_test" + id
	needsCtx := g.testNeedsCtx(w.f, t)
	w.f.needPkg("testing")
	w.directive(t)
	w.line("func " + wrapper + "(t *_s_testing.T) {")
	w.indent++
	args := ""
	if needsCtx {
		args = "_s_context.Background()"
		w.f.needPkg("context")
	}
	w.linef("if _s_err := %s(%s); _s_err != nil {", helper, args)
	w.indent++
	w.line("t.Fatal(_s_err)")
	w.indent--
	w.line("}")
	w.indent--
	w.line("}")
	w.line("")
	w.directive(t)
	sig := "func " + helper + "("
	if needsCtx {
		sig += "_s_ctx _s_context.Context"
		w.f.needPkg("context")
	}
	sig += ") error"
	w.line(sig + " {")
	w.indent++
	w.err = true
	w.results = nil
	if needsCtx {
		w.ctx = "_s_ctx"
	}
	if bodyHasDefer(t.Body) {
		w.deferStack = true
		w.f.needPkg("rt")
		w.line("var _s_d _s_rt.DeferStack")
		w.line("defer _s_d.RunTo(0)")
	}
	var stmts []ast.Stmt
	if t.Body != nil {
		stmts = t.Body.Stmts
	}
	w.line("{")
	w.indent++
	w.bodyContents(t.Body, hasDirectDefer(stmts))
	w.indent--
	w.line("}")
	w.line("return nil")
	w.indent--
	w.line("}")
	w.line("")
}

func (g *generator) testNeedsCtx(f *fileGen, t *ast.TestDecl) bool {
	if t.Body == nil {
		return false
	}
	probe := newWriter(f, "")
	found := false
	ast.Inspect(t.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n := n.(type) {
		case *ast.ParStmt:
			found = true
		case *ast.WithinStmt:
			found = true
		case *ast.ForStmt:
			if n.Par {
				found = true
			}
		case *ast.CallExpr:
			if probe.callNeedsCtx(n) {
				found = true
			}
		}
		return !found
	})
	return found
}

func hasLocalSelf(b *ast.Block) bool {
	if b == nil {
		return false
	}
	found := false
	ast.Inspect(b, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n := n.(type) {
		case *ast.VarDecl:
			for _, name := range n.Names {
				if name == "self" {
					found = true
				}
			}
		case *ast.RangeClause:
			for _, name := range n.Names {
				if name == "self" {
					found = true
				}
			}
		case *ast.Param:
			if n.Name == "self" {
				found = true
			}
		case *ast.Pattern:
			for _, name := range n.Fields {
				if name == "self" {
					found = true
				}
			}
		case *ast.Ident:
			if n.Name == "self" && n.Ref != nil && n.Ref.Kind == ast.RefLocal {
				found = true
			}
		}
		return !found
	})
	return found
}
