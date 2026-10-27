package gen

import (
	"fmt"
	"strings"
	"time"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/token"
)

func (w *writer) expr(e ast.Expr) string {
	if e == nil {
		return ""
	}
	switch e := e.(type) {
	case *ast.Ident:
		return w.ident(e)
	case *ast.BasicLit:
		return w.basicLit(e)
	case *ast.SelectorExpr:
		return w.selector(e)
	case *ast.CallExpr:
		return w.call(e)
	case *ast.IndexExpr:
		return w.expr(e.X) + "[" + w.expr(e.Index) + "]"
	case *ast.SliceExpr:
		low := ""
		high := ""
		if e.Low != nil {
			low = w.expr(e.Low)
		}
		if e.High != nil {
			high = w.expr(e.High)
		}
		return w.expr(e.X) + "[" + low + ":" + high + "]"
	case *ast.UnaryExpr:
		return unaryOp(e.Op) + w.expr(e.X)
	case *ast.BinaryExpr:
		return w.expr(e.X) + " " + binaryOp(e.Op) + " " + w.expr(e.Y)
	case *ast.ParenExpr:
		return "(" + w.expr(e.X) + ")"
	case *ast.CompositeLit:
		return w.composite(e)
	case *ast.FuncLit:
		return w.funcLit(e)
	case *ast.NilLit:
		if e.Type == nil {
			return "nil"
		}
		return "(" + w.typeStr(e.Type) + ")(nil)"
	case *ast.MatchExpr:
		return w.matchExpr(e)
	case *ast.OrExpr:
		return w.expr(e.X)
	}
	return ""
}

func (w *writer) ident(id *ast.Ident) string {
	if id.Ref != nil {
		switch id.Ref.Kind {
		case ast.RefField:
			name := id.Name
			if id.Ref.Field != nil {
				name = id.Ref.Field.Name
			}
			return w.selfRef() + "." + name
		case ast.RefSelf:
			return w.selfRef()
		case ast.RefPackage:
			w.f.usePkg(id.Name)
			return id.Name
		case ast.RefEnumVariant:
			enum := id.Ref.Type
			if enum == nil && id.Ref.Variant != nil {
				enum = id.Ref.Variant.Enum
			}
			return w.enumValue(enum, id.Ref.Variant, "")
		case ast.RefType:
			if id.Ref.TypeVal != nil {
				return w.typeStr(id.Ref.TypeVal)
			}
			if id.Ref.Type != nil {
				return id.Ref.Type.GoName
			}
		}
	}
	if id.Ref != nil && id.Ref.Kind == ast.RefLocal {
		if r, ok := w.renames[id.Name]; ok {
			if r.decl == nil {
				if id.Ref.Decl == nil {
					return r.name
				}
			} else if id.Ref.Decl == r.decl {
				return r.name
			}
		}
	}
	return id.Name
}

func (w *writer) selector(s *ast.SelectorExpr) string {
	if s.Ref == nil {
		if id, ok := s.X.(*ast.Ident); ok && id.Ref == nil && w.f.pkg.Imports[id.Name] != nil {
			w.f.usePkg(id.Name)
			return id.Name + "." + s.Name
		}
	}
	if s.Ref != nil {
		switch s.Ref.Kind {
		case ast.RefPackage:
			w.f.usePkg(s.Name)
			return w.expr(s.X) + "." + s.Name
		case ast.RefEnumVariant:
			enum := s.Ref.Type
			v := s.Ref.Variant
			if enum == nil && v != nil {
				enum = v.Enum
			}
			return w.enumValue(enum, v, w.packageOf(s.X))
		case ast.RefField:
			name := s.Name
			if s.Ref.Field != nil {
				name = s.Ref.Field.Name
			}
			return w.expr(s.X) + "." + name
		}
	}
	return w.expr(s.X) + "." + s.Name
}

func (w *writer) packageOf(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Ref != nil && x.Ref.Kind == ast.RefPackage {
			return x.Name
		}
	case *ast.SelectorExpr:
		return w.packageOf(x.X)
	case *ast.ParenExpr:
		return w.packageOf(x.X)
	}
	return ""
}

func (w *writer) enumValue(enum *ast.TypeDecl, v *ast.Variant, qual string) string {
	if v == nil {
		return ""
	}
	if enum == nil {
		enum = v.Enum
	}
	if qual != "" {
		w.f.usePkg(qual)
	}
	et := qualified(qual, w.f.g.enumName(enum))
	vt := qualified(qual, w.f.g.variantName(v))
	return et + "(&" + vt + "{})"
}

func (w *writer) basicLit(e *ast.BasicLit) string {
	if e.Kind == token.Duration {
		if d, err := time.ParseDuration(e.Value); err == nil {
			w.f.needPkg("time")
			return fmt.Sprintf("_s_time.Duration(%d)", int64(d))
		}
	}
	return e.Value
}

func (w *writer) call(c *ast.CallExpr) string {
	if name, ok := builtinName(c); ok {
		var args []string
		for _, a := range c.Args {
			args = append(args, w.expr(a))
		}
		return w.builtinCall(name, c, args)
	}
	if isConversion(c) {
		target := w.conversionTarget(c)
		var arg string
		if len(c.Args) > 0 {
			arg = w.expr(c.Args[0])
		}
		return w.typeStr(target) + "(" + arg + ")"
	}
	fun, prefix, handled := w.callTarget(c)
	if len(c.TypeArgs) > 0 {
		parts := make([]string, 0, len(c.TypeArgs))
		for _, t := range c.TypeArgs {
			parts = append(parts, w.typeStr(t))
		}
		fun += "[" + strings.Join(parts, ", ") + "]"
	}
	args := append([]string{}, prefix...)
	for _, a := range c.Args {
		args = append(args, w.expr(a))
	}
	if !handled && w.callNeedsCtx(c) {
		args = append([]string{w.currentCtx()}, args...)
	}
	if c.Ellipsis && len(args) > 0 {
		args[len(args)-1] += "..."
	}
	return fun + "(" + strings.Join(args, ", ") + ")"
}

func builtinName(c *ast.CallExpr) (string, bool) {
	if c.Info != nil && c.Info.Kind == ast.CallBuiltin && c.Info.Builtin != "" {
		return c.Info.Builtin, true
	}
	if c.Info != nil && c.Info.Kind != ast.CallUnknown && c.Info.Kind != ast.CallBuiltin {
		return "", false
	}
	id, ok := c.Fun.(*ast.Ident)
	if !ok {
		return "", false
	}
	if id.Ref != nil && id.Ref.Kind != ast.RefBuiltin {
		return "", false
	}
	switch id.Name {
	case "len", "cap", "append", "copy", "delete", "has", "panic", "assert", "assert_eq", "min", "max", "runes", "bytes":
		return id.Name, true
	}
	return "", false
}

func isConversion(c *ast.CallExpr) bool {
	if c.Info != nil && c.Info.Kind == ast.CallConversion {
		return true
	}
	if c.Info != nil && c.Info.Kind != ast.CallUnknown {
		return false
	}
	switch f := c.Fun.(type) {
	case *ast.Ident:
		if f.Ref != nil && f.Ref.Kind == ast.RefType {
			return true
		}
		return f.Ref == nil && isStrizhBuiltinType(f.Name)
	case *ast.SelectorExpr:
		return f.Ref != nil && f.Ref.Kind == ast.RefType
	}
	return false
}

func (w *writer) conversionTarget(c *ast.CallExpr) ast.Type {
	if c.Info != nil && len(c.Info.ParamTypes) > 0 {
		return c.Info.ParamTypes[0]
	}
	switch f := c.Fun.(type) {
	case *ast.Ident:
		if f.Ref != nil {
			if f.Ref.TypeVal != nil {
				return f.Ref.TypeVal
			}
			if f.Ref.Type != nil {
				return &ast.NamedType{Name: f.Name, Decl: f.Ref.Type, GoName: f.Ref.Type.GoName}
			}
		}
		if f.Ref == nil && isStrizhBuiltinType(f.Name) {
			return &ast.NamedType{Name: f.Name}
		}
	case *ast.SelectorExpr:
		if f.Ref != nil && f.Ref.TypeVal != nil {
			return f.Ref.TypeVal
		}
	}
	return nil
}

func (w *writer) builtinCall(name string, c *ast.CallExpr, args []string) string {
	switch name {
	case "assert":
		w.f.needPkg("rt")
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		return "_s_rt.Assert(" + arg + ", " + w.posLiteral(c) + ")"
	case "assert_eq":
		w.f.needPkg("rt")
		got := ""
		want := ""
		if len(args) > 0 {
			got = args[0]
		}
		if len(args) > 1 {
			want = args[1]
		}
		return "_s_rt.AssertEq(" + got + ", " + want + ", " + w.posLiteral(c) + ")"
	case "has":
		w.f.needPkg("rt")
		if len(args) == 2 {
			return "_s_rt.Has(" + args[0] + ", " + args[1] + ")"
		}
		return "_s_rt.Has(" + strings.Join(args, ", ") + ")"
	case "runes":
		if len(args) > 0 {
			return "[]rune(" + args[0] + ")"
		}
		return "[]rune{}"
	case "bytes":
		if len(args) > 0 {
			return "[]byte(" + args[0] + ")"
		}
		return "[]byte{}"
	}
	if c.Ellipsis && len(args) > 0 {
		args[len(args)-1] += "..."
	}
	return name + "(" + strings.Join(args, ", ") + ")"
}

func (w *writer) posLiteral(n ast.Node) string {
	return fmt.Sprintf("%q", w.f.g.pos(n))
}

func (w *writer) callTarget(c *ast.CallExpr) (string, []string, bool) {
	fd := calledDecl(c)
	if fd == nil || !fd.NeedsCtx {
		return w.expr(c.Fun), nil, false
	}
	if w.ctx == "" || !w.isLocalDecl(fd) {
		return w.expr(c.Fun), nil, true
	}
	name := ctxFuncName(fd)
	if sel, ok := c.Fun.(*ast.SelectorExpr); ok && fd.Owner != nil {
		recv := w.expr(sel.X)
		self := false
		if id, ok := sel.X.(*ast.Ident); ok && id.Ref != nil && id.Ref.Kind == ast.RefSelf {
			self = true
		}
		if !self && !isPointerType(w.f.g.p.ExprTypes[sel.X]) {
			recv = "&" + recv
		}
		return name, []string{recv, w.currentCtx()}, true
	}
	if fd.Owner == nil {
		return name, []string{w.currentCtx()}, true
	}
	return w.expr(c.Fun), nil, true
}

func (w *writer) isLocalDecl(fd *ast.FuncDecl) bool {
	if fd.Owner != nil {
		return w.f.pkg.Types[fd.Owner.Name] == fd.Owner
	}
	for _, f := range w.f.pkg.Funcs {
		if f == fd {
			return true
		}
	}
	return false
}

func calledDecl(c *ast.CallExpr) *ast.FuncDecl {
	switch f := c.Fun.(type) {
	case *ast.Ident:
		if f.Ref != nil {
			return f.Ref.Func
		}
	case *ast.SelectorExpr:
		if f.Ref != nil {
			return f.Ref.Func
		}
	}
	if c.Info != nil {
		return c.Info.Fn
	}
	return nil
}

func isPointerType(t ast.Type) bool {
	switch t := t.(type) {
	case *ast.PointerType:
		return true
	case *ast.NamedType:
		if t.Decl != nil && t.Decl.Kind == ast.TypeAlias {
			return isPointerType(t.Decl.Target)
		}
	}
	return false
}

func (w *writer) callNeedsCtx(c *ast.CallExpr) bool {
	if _, ok := c.Fun.(*ast.FuncLit); ok {
		return false
	}
	switch f := c.Fun.(type) {
	case *ast.Ident:
		if f.Ref != nil && f.Ref.Func != nil && f.Ref.Func.NeedsCtx {
			return true
		}
	case *ast.SelectorExpr:
		if f.Ref != nil && f.Ref.Func != nil && f.Ref.Func.NeedsCtx {
			return true
		}
	}
	if c.Info == nil {
		return false
	}
	if c.Info.NeedsCtx {
		return true
	}
	if c.Info.Fn != nil && c.Info.Fn.NeedsCtx {
		return true
	}
	return false
}

func unaryOp(k token.Kind) string {
	switch k {
	case token.Tilde:
		return "^"
	default:
		return k.String()
	}
}

func binaryOp(k token.Kind) string {
	if k >= token.PlusAssign && k <= token.ShrAssign {
		return k.String()
	}
	switch k {
	case token.AndAnd:
		return "&&"
	case token.OrOr:
		return "||"
	}
	return k.String()
}

func (w *writer) composite(e *ast.CompositeLit) string {
	if e.Variant != nil {
		qual := ""
		if n, ok := e.Type.(*ast.NamedType); ok && n.PkgAlias != "" {
			qual = n.PkgAlias
		}
		enum := e.Variant.Enum
		if enum == nil {
			if n, ok := e.Type.(*ast.NamedType); ok {
				enum = n.Decl
			}
		}
		et := qualified(qual, w.f.g.enumName(enum))
		vt := qualified(qual, w.f.g.variantName(e.Variant))
		if qual != "" {
			w.f.usePkg(qual)
		}
		return et + "(&" + vt + "{" + w.entries(e) + "})"
	}
	return w.typeStr(e.Type) + "{" + w.entries(e) + "}"
}

func (w *writer) entries(e *ast.CompositeLit) string {
	parts := make([]string, 0, len(e.Entries))
	for _, en := range e.Entries {
		val := w.expr(en.Value)
		if en.KeyExpr != nil {
			parts = append(parts, w.expr(en.KeyExpr)+": "+val)
			continue
		}
		if en.Key != "" || en.Field != nil {
			key := en.Key
			if en.Field != nil {
				key = en.Field.Name
			}
			parts = append(parts, key+": "+val)
			continue
		}
		parts = append(parts, val)
	}
	return strings.Join(parts, ", ")
}

func (w *writer) funcLit(lit *ast.FuncLit) string {
	var params []*ast.Param
	params = append(params, lit.Params...)
	var types []ast.Type
	types = append(types, lit.ParamTypes...)
	paramStr := w.f.g.paramsStr(w.f, params, types, false)
	res := w.f.g.resultStr(w.f, lit.Results, lit.Err)
	sub := w.sub()
	sub.results = lit.Results
	sub.err = lit.Err
	if lit.GoCallback {
		sub.ctx = "_s_context.Background()"
	}
	sub.par = nil
	sub.loops = nil
	if lit.Arrow != nil {
		sub.indent = 1
		if lit.Err {
			if len(lit.Results) > 0 {
				sub.line("return " + sub.expr(lit.Arrow) + ", nil")
			} else {
				sub.line("return " + sub.expr(lit.Arrow))
			}
		} else {
			sub.line("return " + sub.expr(lit.Arrow))
		}
	} else if lit.Body != nil {
		sub.indent = 1
		if bodyHasDefer(lit.Body) {
			sub.deferStack = true
			sub.f.needPkg("rt")
			sub.line("var _s_d _s_rt.DeferStack")
			sub.line("defer _s_d.RunTo(0)")
		}
		sub.line("{")
		sub.indent++
		sub.bodyContents(lit.Body, hasDirectDefer(lit.Body.Stmts))
		sub.indent--
		sub.line("}")
		if lit.Err && len(lit.Results) == 0 {
			sub.line("return nil")
		}
		if len(lit.Results) > 0 {
			sub.line(`panic("unreachable")`)
		}
	}
	return "func(" + paramStr + ")" + res + " {\n" + sub.buf.String() + "}"
}
