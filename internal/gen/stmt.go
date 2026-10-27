package gen

import (
	"strconv"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/token"
)

func (w *writer) bodyContents(b *ast.Block, mark bool) {
	if b == nil {
		return
	}
	markName := ""
	if mark && w.deferStack {
		markName = w.next("mark")
		w.linef("%s := _s_d.Mark()", markName)
	}
	w.genStmts(b.Stmts)
	if markName != "" {
		w.linef("_s_d.RunTo(%s)", markName)
	}
}

func (w *writer) genStmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		w.stmt(s)
	}
}

func (w *writer) stmt(s ast.Stmt) {
	if s == nil {
		return
	}
	if _, ok := s.(*ast.EmptyStmt); ok {
		return
	}
	w.directive(s)
	switch s := s.(type) {
	case *ast.Block:
		w.writeIndent()
		w.buf.WriteString("{\n")
		w.indent++
		w.bodyContents(s, hasDirectDefer(s.Stmts))
		w.indent--
		w.writeIndent()
		w.buf.WriteString("}\n")
	case *ast.DeclStmt:
		w.declStmt(s.Decl)
	case *ast.AssignStmt:
		w.assignStmt(s)
	case *ast.ExprStmt:
		if or, ok := s.X.(*ast.OrExpr); ok {
			w.orPrelude(or, false)
			return
		}
		if m, ok := s.X.(*ast.MatchExpr); ok {
			w.matchSwitch(m, false)
			return
		}
		w.line(w.expr(s.X))
	case *ast.IfStmt:
		w.ifClause(s, true)
	case *ast.ForStmt:
		w.forStmt(s)
	case *ast.ReturnStmt:
		w.returnStmt(s)
	case *ast.BreakStmt:
		w.breakStmt("break")
	case *ast.ContinueStmt:
		w.breakStmt("continue")
	case *ast.DeferStmt:
		w.deferStmt(s)
	case *ast.ParStmt:
		w.parStmt(s)
	case *ast.WithinStmt:
		w.withinStmt(s)
	}
}

func (w *writer) ifClause(s *ast.IfStmt, top bool) {
	if top {
		w.writeIndent()
	}
	w.buf.WriteString("if ")
	w.buf.WriteString(w.expr(s.Cond))
	w.buf.WriteString(" {\n")
	w.indent++
	w.bodyContents(s.Then, hasDirectDefer(s.Then.Stmts))
	w.indent--
	w.writeIndent()
	w.buf.WriteString("}")
	switch e := s.Else.(type) {
	case nil:
		w.buf.WriteString("\n")
	case *ast.Block:
		w.buf.WriteString(" else {\n")
		w.indent++
		w.bodyContents(e, hasDirectDefer(e.Stmts))
		w.indent--
		w.writeIndent()
		w.buf.WriteString("}\n")
	case *ast.IfStmt:
		w.buf.WriteString(" else ")
		w.ifClause(e, false)
	default:
		w.buf.WriteString("\n")
	}
}

func (w *writer) declStmt(d *ast.VarDecl) {
	if d == nil {
		return
	}
	prefix := "var"
	if d.Const {
		prefix = "const"
	}
	if len(d.Values) == 1 {
		if o, isOr := d.Values[0].(*ast.OrExpr); isOr {
			value := w.orPrelude(o, true)
			if value == "" {
				return
			}
			names := strings.Join(d.Names, ", ")
			if allBlank(d.Names) {
				w.linef("_ = %s", value)
			} else {
				w.linef("%s %s = %s", prefix, names, value)
			}
			return
		}
	}
	if len(d.Values) == 0 {
		return
	}
	values := make([]string, 0, len(d.Values))
	for _, v := range d.Values {
		if _, ok := v.(*ast.OrExpr); ok {
			w.f.g.errAt(v, "or is not supported in this declaration")
			continue
		}
		values = append(values, w.expr(v))
	}
	names := strings.Join(d.Names, ", ")
	if allBlank(d.Names) {
		if len(values) == 1 {
			w.linef("_ = %s", values[0])
		} else {
			w.linef("_, _ = %s", strings.Join(values, ", "))
		}
		return
	}
	w.linef("%s %s = %s", prefix, names, strings.Join(values, ", "))
}

func allBlank(names []string) bool {
	for _, n := range names {
		if n != "_" {
			return false
		}
	}
	return true
}

func (w *writer) assignStmt(s *ast.AssignStmt) {
	lhs := make([]string, 0, len(s.Lhs))
	for _, e := range s.Lhs {
		lhs = append(lhs, w.expr(e))
	}
	if len(s.Rhs) == 1 {
		if or, ok := s.Rhs[0].(*ast.OrExpr); ok {
			value := w.orPrelude(or, true)
			if value == "" {
				return
			}
			w.linef("%s = %s", strings.Join(lhs, ", "), value)
			return
		}
	}
	rhs := make([]string, 0, len(s.Rhs))
	for _, e := range s.Rhs {
		if _, ok := e.(*ast.OrExpr); ok {
			w.f.g.errAt(e, "or is not supported in this assignment")
			continue
		}
		rhs = append(rhs, w.expr(e))
	}
	op := "="
	if s.Op != token.Assign {
		op = s.Op.String()
	}
	w.linef("%s %s %s", strings.Join(lhs, ", "), op, strings.Join(rhs, ", "))
}

func (w *writer) returnStmt(s *ast.ReturnStmt) {
	if len(s.Results) == 1 {
		if or, ok := s.Results[0].(*ast.OrExpr); ok {
			results := 1
			if call, ok := or.X.(*ast.CallExpr); ok && call.Info != nil {
				results = call.Info.Results
			}
			value := w.orPrelude(or, results > 0)
			if value == "" {
				w.ret(nil)
			} else {
				w.ret([]string{value})
			}
			return
		}
	}
	vals := make([]string, 0, len(s.Results))
	for _, e := range s.Results {
		vals = append(vals, w.expr(e))
	}
	if len(vals) == 1 && w.err && w.isErrorExpr(s.Results[0]) {
		w.returnErr(vals[0])
		return
	}
	if len(vals) == 1 {
		if call, ok := s.Results[0].(*ast.CallExpr); ok {
			if (callResults(call) > 1 || isFallibleCall(call)) && callResults(call) == len(w.results) {
				w.retRaw(vals[0])
				return
			}
		}
	}
	w.ret(vals)
}

func (w *writer) isErrorExpr(e ast.Expr) bool {
	return isErrorType(w.f.g.p.ExprTypes[e]) || isErrorCall(e)
}

func isErrorType(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.NamedType:
		return t.Package == "" && t.Name == "error"
	case *ast.GoType:
		return t.Text == "error"
	}
	return false
}

func isErrorCall(e ast.Expr) bool {
	c, ok := e.(*ast.CallExpr)
	return ok && c.Info != nil && c.Info.HasError && c.Info.Results == 0
}

func isFallibleCall(c *ast.CallExpr) bool {
	return c.Info != nil && c.Info.HasError
}

func callResults(c *ast.CallExpr) int {
	if c.Info == nil {
		return 0
	}
	return c.Info.Results
}

func (w *writer) ret(vals []string) {
	if w.deferStack {
		w.line("_s_d.RunTo(0)")
	}
	if len(vals) == 1 && vals[0] == "" {
		vals = nil
	}
	if w.err {
		n := len(w.results)
		if len(vals) == 0 {
			if n == 0 {
				w.line("return nil")
				return
			}
			w.line("return " + w.zeroResults() + ", nil")
			return
		}
		if len(vals) == n {
			w.line("return " + strings.Join(vals, ", ") + ", nil")
			return
		}
		w.line("return " + strings.Join(vals, ", "))
		return
	}
	if len(vals) == 0 {
		w.line("return")
		return
	}
	w.line("return " + strings.Join(vals, ", "))
}

func (w *writer) zeroResults() string {
	parts := make([]string, 0, len(w.results))
	for _, t := range w.results {
		parts = append(parts, "*new("+w.typeStr(t)+")")
	}
	return strings.Join(parts, ", ")
}

func (w *writer) retRaw(value string) {
	if w.deferStack {
		w.line("_s_d.RunTo(0)")
	}
	w.line("return " + value)
}

func (w *writer) returnErr(errName string) {
	if w.deferStack {
		w.line("_s_d.RunTo(0)")
	}
	if !w.err {
		w.linef("panic(%s)", errName)
		return
	}
	if len(w.results) == 0 {
		w.linef("return %s", errName)
		return
	}
	w.linef("return %s, %s", w.zeroResults(), errName)
}

func (w *writer) breakStmt(kind string) {
	if w.deferStack && len(w.loops) > 0 {
		w.linef("_s_d.RunTo(%s)", w.loops[len(w.loops)-1])
	}
	w.line(kind)
}

func (w *writer) orPrelude(or *ast.OrExpr, wantValue bool) string {
	results := 1
	var callStr string
	if call, ok := or.X.(*ast.CallExpr); ok {
		if call.Info != nil {
			results = call.Info.Results
		}
		callStr = w.call(call)
	} else {
		callStr = w.expr(or.X)
	}
	errName := w.next("err")
	valueName := ""
	switch {
	case results > 0 && wantValue:
		valueName = w.next("v")
		w.linef("%s, %s := %s", valueName, errName, callStr)
	case results > 0:
		w.linef("_, %s := %s", errName, callStr)
	default:
		w.linef("%s := %s", errName, callStr)
	}
	w.orError(or, errName)
	return valueName
}

func (w *writer) orError(or *ast.OrExpr, errName string) {
	switch or.Mode {
	case ast.OrReturn:
		w.linef("if %s != nil {", errName)
		w.indent++
		w.propagateParError(errName)
		w.indent--
		w.line("}")
	case ast.OrHandle:
		w.linef("if %s != nil {", errName)
		w.indent++
		if or.Name != "" && or.Name != "_" {
			w.linef("%s := %s", or.Name, errName)
			w.linef("_ = %s", or.Name)
		}
		w.line("{")
		w.indent++
		if or.Block != nil {
			w.bodyContents(or.Block, hasDirectDefer(or.Block.Stmts))
		}
		w.indent--
		w.line("}")
		w.indent--
		w.line("}")
	case ast.OrPanic:
		arg := errName
		if or.PanicArg != nil {
			arg = w.expr(or.PanicArg)
		}
		w.linef("if %s != nil {", errName)
		w.indent++
		if w.par != nil {
			if w.deferStack {
				w.line("_s_d.RunTo(0)")
			}
			w.f.needPkg("rt")
			w.linef("%s.Add(%s, _s_rt.AsError(%s))", w.par.pe, w.par.ord, arg)
			if w.par.cancel != "" {
				w.linef("%s()", w.par.cancel)
			}
			w.line("return")
		} else {
			w.linef("panic(%s)", arg)
		}
		w.indent--
		w.line("}")
	}
}

func (w *writer) propagateParError(errName string) {
	if w.par != nil {
		if w.deferStack {
			w.line("_s_d.RunTo(0)")
		}
		if w.par.pe != "" {
			w.linef("%s.Add(%s, %s)", w.par.pe, w.par.ord, errName)
		}
		if w.par.cancel != "" {
			w.linef("%s()", w.par.cancel)
		}
		w.line("return")
		return
	}
	w.returnErr(errName)
}

func (w *writer) deferStmt(s *ast.DeferStmt) {
	c := s.Call
	if c == nil {
		return
	}
	w.f.needPkg("rt")
	w.deferStack = true
	var args []string
	var callStr string
	if isConversion(c) {
		for _, a := range c.Args {
			name := w.next("x")
			w.linef("%s := %s", name, w.expr(a))
			args = append(args, name)
		}
		callStr = w.typeStr(w.conversionTarget(c)) + "(" + strings.Join(args, ", ") + ")"
	} else if name, ok := builtinName(c); ok {
		for _, a := range c.Args {
			tmp := w.next("x")
			w.linef("%s := %s", tmp, w.expr(a))
			args = append(args, tmp)
		}
		if c.Ellipsis && len(args) > 0 {
			args[len(args)-1] += "..."
		}
		callStr = w.builtinCall(name, c, args)
	} else {
		fun, prefix, handled := w.callTarget(c)
		if len(c.TypeArgs) > 0 {
			parts := make([]string, 0, len(c.TypeArgs))
			for _, t := range c.TypeArgs {
				parts = append(parts, w.typeStr(t))
			}
			fun += "[" + strings.Join(parts, ", ") + "]"
		}
		funName := w.next("f")
		w.linef("%s := %s", funName, fun)
		callStr = funName
		var pre []string
		for _, p := range prefix {
			tmp := w.next("x")
			w.linef("%s := %s", tmp, p)
			pre = append(pre, tmp)
		}
		if len(prefix) == 0 && !handled && w.callNeedsCtx(c) {
			tmp := w.next("x")
			w.linef("%s := %s", tmp, w.currentCtx())
			pre = append(pre, tmp)
		}
		args = append(args, pre...)
		for _, a := range c.Args {
			tmp := w.next("x")
			w.linef("%s := %s", tmp, w.expr(a))
			args = append(args, tmp)
		}
		if c.Ellipsis && len(args) > 0 {
			args[len(args)-1] += "..."
		}
		callStr += "(" + strings.Join(args, ", ") + ")"
	}
	w.linef("_s_d.Push(func() { %s })", callStr)
}

func (w *writer) forStmt(s *ast.ForStmt) {
	if s.Par {
		w.parForStmt(s)
		return
	}
	if s.Range != nil {
		w.rangeHead(s.Range)
	} else {
		init := w.initString(s.Init)
		cond := ""
		if s.Cond != nil {
			cond = w.expr(s.Cond)
		}
		post := w.postString(s.Post)
		switch {
		case init == "" && post == "" && cond != "":
			w.linef("for %s {", cond)
		case init == "" && post == "":
			w.line("for {")
		default:
			w.linef("for %s; %s; %s {", init, cond, post)
		}
	}
	w.indent++
	mark := ""
	if w.deferStack {
		mark = w.next("mark")
		w.linef("%s := _s_d.Mark()", mark)
		w.loops = append(w.loops, mark)
	}
	if s.Body != nil {
		w.genStmts(s.Body.Stmts)
	}
	if mark != "" {
		w.linef("_s_d.RunTo(%s)", mark)
		w.loops = w.loops[:len(w.loops)-1]
	}
	w.indent--
	w.line("}")
}

func (w *writer) rangeHead(r *ast.RangeClause) {
	if r == nil {
		w.line("for {")
		return
	}
	x := w.expr(r.X)
	if allBlank(r.Names) || len(r.Names) == 0 {
		w.linef("for range %s {", x)
		return
	}
	if len(r.Names) == 1 {
		if r.IsMap {
			w.linef("for %s := range %s {", r.Names[0], x)
		} else {
			w.linef("for _, %s := range %s {", r.Names[0], x)
		}
		return
	}
	w.linef("for %s, %s := range %s {", r.Names[0], r.Names[1], x)
}

func (w *writer) initString(s ast.Stmt) string {
	switch s := s.(type) {
	case nil:
		return ""
	case *ast.DeclStmt:
		if s.Decl == nil {
			return ""
		}
		if len(s.Decl.Values) == 1 {
			if or, ok := s.Decl.Values[0].(*ast.OrExpr); ok {
				value := w.orPrelude(or, true)
				if value == "" {
					return ""
				}
				if allBlank(s.Decl.Names) {
					return "_ = " + value
				}
				return strings.Join(s.Decl.Names, ", ") + " := " + value
			}
		}
		values := make([]string, 0, len(s.Decl.Values))
		for _, v := range s.Decl.Values {
			if _, ok := v.(*ast.OrExpr); ok {
				w.f.g.errAt(v, "or is not supported in a for initializer")
				continue
			}
			values = append(values, w.expr(v))
		}
		if allBlank(s.Decl.Names) {
			if len(values) == 1 {
				return "_ = " + values[0]
			}
			return "_, _ = " + strings.Join(values, ", ")
		}
		return strings.Join(s.Decl.Names, ", ") + " := " + strings.Join(values, ", ")
	case *ast.AssignStmt:
		if len(s.Rhs) == 1 {
			if _, ok := s.Rhs[0].(*ast.OrExpr); ok {
				w.f.g.errAt(s, "or is not supported in a for initializer")
			}
		}
		lhs := make([]string, 0, len(s.Lhs))
		for _, e := range s.Lhs {
			lhs = append(lhs, w.expr(e))
		}
		rhs := make([]string, 0, len(s.Rhs))
		for _, e := range s.Rhs {
			rhs = append(rhs, w.expr(e))
		}
		op := "="
		if s.Op != token.Assign {
			op = s.Op.String()
		}
		return strings.Join(lhs, ", ") + " " + op + " " + strings.Join(rhs, ", ")
	case *ast.ExprStmt:
		return w.expr(s.X)
	}
	return ""
}

func (w *writer) postString(s ast.Stmt) string {
	switch s := s.(type) {
	case nil:
		return ""
	case *ast.AssignStmt:
		if len(s.Rhs) == 1 {
			if _, ok := s.Rhs[0].(*ast.OrExpr); ok {
				w.f.g.errAt(s, "or is not supported in a for post statement")
			}
		}
		lhs := make([]string, 0, len(s.Lhs))
		for _, e := range s.Lhs {
			lhs = append(lhs, w.expr(e))
		}
		rhs := make([]string, 0, len(s.Rhs))
		for _, e := range s.Rhs {
			rhs = append(rhs, w.expr(e))
		}
		op := "="
		if s.Op != token.Assign {
			op = s.Op.String()
		}
		return strings.Join(lhs, ", ") + " " + op + " " + strings.Join(rhs, ", ")
	case *ast.ExprStmt:
		return w.expr(s.X)
	case *ast.DeclStmt:
		if s.Decl == nil || len(s.Decl.Values) == 0 {
			return ""
		}
		values := make([]string, 0, len(s.Decl.Values))
		for _, v := range s.Decl.Values {
			values = append(values, w.expr(v))
		}
		return strings.Join(s.Decl.Names, ", ") + " := " + strings.Join(values, ", ")
	}
	return ""
}

func (w *writer) parStmt(s *ast.ParStmt) {
	if s.Body == nil {
		return
	}
	w.line("{")
	w.indent++
	wg := w.next("wg")
	w.linef("var %s _s_sync.WaitGroup", wg)
	w.f.needPkg("sync")
	ctx := w.next("ctx")
	cancel := w.next("cancel")
	pe := w.next("pe")
	w.linef("%s, %s := _s_context.WithCancel(%s)", ctx, cancel, w.currentCtx())
	w.linef("_ = %s", ctx)
	w.f.needPkg("context")
	w.linef("var %s _s_rt.ParErrors", pe)
	w.f.needPkg("rt")
	w.linef("%s.Add(%d)", wg, len(s.Body.Stmts))
	for i, st := range s.Body.Stmts {
		sub := w.sub()
		sub.ctx = ctx
		sub.results = nil
		sub.err = false
		sub.deferStack = true
		sub.par = &parRef{pe: pe, ord: strconv.Itoa(i), cancel: cancel}
		sub.line("var _s_d _s_rt.DeferStack")
		sub.line("defer _s_d.RunTo(0)")
		sub.stmt(st)
		w.line("go func() {")
		w.indent++
		w.linef("defer %s.Done()", wg)
		w.linef("defer func() { if r := recover(); r != nil { %s.Add(%s, _s_rt.NewPanicError(r)); %s() } }()", pe, sub.par.ord, cancel)
		w.buf.WriteString(sub.buf.String())
		w.indent--
		w.line("}()")
	}
	w.linef("%s.Wait()", wg)
	errName := w.next("err")
	w.linef("if %s := %s.First(); %s != nil {", errName, pe, errName)
	w.indent++
	w.linef("%s()", cancel)
	w.propagateParError(errName)
	w.indent--
	w.line("}")
	w.linef("%s()", cancel)
	w.indent--
	w.line("}")
}

func (w *writer) parForStmt(s *ast.ForStmt) {
	w.line("{")
	w.indent++
	wg := w.next("wg")
	sem := w.next("sem")
	w.linef("var %s _s_sync.WaitGroup", wg)
	w.f.needPkg("sync")
	limit := "_s_runtime.GOMAXPROCS(0)"
	if s.Limit != nil {
		limit = w.expr(s.Limit)
	} else {
		w.f.needPkg("runtime")
	}
	w.linef("%s := make(chan struct{}, %s)", sem, limit)
	ctx := w.next("ctx")
	cancel := w.next("cancel")
	pe := w.next("pe")
	w.linef("%s, %s := _s_context.WithCancel(%s)", ctx, cancel, w.currentCtx())
	w.linef("_ = %s", ctx)
	w.f.needPkg("context")
	w.linef("var %s _s_rt.ParErrors", pe)
	w.f.needPkg("rt")
	isMap := s.Range != nil && s.Range.IsMap
	ord := ""
	if !isMap {
		ord = w.next("ord")
		w.linef("%s := 0", ord)
	}
	init := w.initString(s.Init)
	var head string
	if s.Range != nil {
		x := w.expr(s.Range.X)
		switch {
		case allBlank(s.Range.Names) || len(s.Range.Names) == 0:
			head = "for range " + x + " {"
		case len(s.Range.Names) == 1 && s.Range.IsMap:
			head = "for " + s.Range.Names[0] + " := range " + x + " {"
		case len(s.Range.Names) == 1:
			head = "for _, " + s.Range.Names[0] + " := range " + x + " {"
		default:
			head = "for " + s.Range.Names[0] + ", " + s.Range.Names[1] + " := range " + x + " {"
		}
	} else {
		cond := ""
		if s.Cond != nil {
			cond = w.expr(s.Cond)
		}
		post := w.postString(s.Post)
		switch {
		case init == "" && post == "" && cond != "":
			head = "for " + cond + " {"
		case init == "" && post == "":
			head = "for {"
		default:
			head = "for " + init + "; " + cond + "; " + post + " {"
		}
	}
	w.line(head)
	w.indent++
	w.linef("%s <- struct{}{}", sem)
	var order []rename
	var orderNames []string
	if s.Range != nil {
		for _, n := range s.Range.Names {
			if n == "_" {
				continue
			}
			capture := w.next("i")
			w.linef("%s := %s", capture, n)
			w.linef("_ = %s", capture)
			order = append(order, rename{name: capture})
			orderNames = append(orderNames, n)
		}
	} else if d, ok := s.Init.(*ast.DeclStmt); ok && d.Decl != nil {
		for _, n := range d.Decl.Names {
			if n == "_" {
				continue
			}
			capture := w.next("i")
			w.linef("%s := %s", capture, n)
			w.linef("_ = %s", capture)
			order = append(order, rename{name: capture, decl: d.Decl})
			orderNames = append(orderNames, n)
		}
	} else if a, ok := s.Init.(*ast.AssignStmt); ok {
		for _, l := range a.Lhs {
			id, ok := l.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			capture := w.next("i")
			w.linef("%s := %s", capture, id.Name)
			w.linef("_ = %s", capture)
			var decl ast.Node
			if id.Ref != nil {
				decl = id.Ref.Decl
			}
			order = append(order, rename{name: capture, decl: decl})
			orderNames = append(orderNames, id.Name)
		}
	}
	ordName := "-1"
	if !isMap {
		ordName = w.next("ord")
		w.linef("%s := %s", ordName, ord)
		w.linef("%s++", ord)
	}
	w.linef("%s.Add(1)", wg)
	sub := w.sub()
	sub.ctx = ctx
	sub.results = nil
	sub.err = false
	sub.deferStack = true
	sub.par = &parRef{pe: pe, ord: ordName, cancel: cancel}
	sub.line("var _s_d _s_rt.DeferStack")
	sub.line("defer _s_d.RunTo(0)")
	if s.Body != nil {
		for i, r := range order {
			sub.renames[orderNames[i]] = r
		}
		sub.genStmts(s.Body.Stmts)
	}
	w.line("go func() {")
	w.indent++
	w.linef("defer %s.Done()", wg)
	w.linef("defer func() { <-%s }()", sem)
	w.linef("defer func() { if r := recover(); r != nil { %s.Add(%s, _s_rt.NewPanicError(r)); %s() } }()", pe, ordName, cancel)
	w.buf.WriteString(sub.buf.String())
	w.indent--
	w.line("}()")
	w.indent--
	w.line("}")
	w.linef("%s.Wait()", wg)
	errName := w.next("err")
	w.linef("if %s := %s.First(); %s != nil {", errName, pe, errName)
	w.indent++
	w.linef("%s()", cancel)
	w.propagateParError(errName)
	w.indent--
	w.line("}")
	w.linef("%s()", cancel)
	w.indent--
	w.line("}")
}

func (w *writer) withinStmt(s *ast.WithinStmt) {
	w.f.needPkg("context")
	w.f.needPkg("rt")
	w.line("{")
	w.indent++
	ctx := w.next("ctx")
	cancel := w.next("cancel")
	mark := w.next("mark")
	w.linef("%s, %s := _s_context.WithTimeout(%s, %s)", ctx, cancel, w.currentCtx(), w.expr(s.Duration))
	w.linef("_ = %s", ctx)
	w.linef("%s := _s_d.Mark()", mark)
	w.linef("_s_d.Push(%s)", cancel)
	old := w.ctx
	w.ctx = ctx
	if s.Body != nil {
		w.genStmts(s.Body.Stmts)
	}
	w.ctx = old
	w.linef("_s_d.RunTo(%s)", mark)
	w.indent--
	w.line("}")
}

func bodyHasDefer(b *ast.Block) bool {
	if b == nil {
		return false
	}
	found := false
	ast.Inspect(b, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		switch n.(type) {
		case *ast.DeferStmt, *ast.WithinStmt:
			found = true
			return false
		}
		return true
	})
	return found
}

func hasDirectDefer(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.DeferStmt); ok {
			return true
		}
	}
	return false
}
