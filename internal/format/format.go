package format

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/token"
)

func File(f *ast.File) []byte {
	p := &printer{comments: f.Comments}
	for _, u := range f.Uses {
		p.use(u)
	}
	if len(f.Uses) > 0 && len(f.Decls) > 0 {
		p.blank()
	}
	for i, d := range f.Decls {
		if i > 0 {
			p.blank()
		}
		p.decl(d)
	}
	p.flush(len(f.Src) + 1)
	p.trim()
	return p.buf.Bytes()
}

type printer struct {
	buf      bytes.Buffer
	indent   int
	comments []*ast.Comment
	ci       int
	lastLine int
	atStart  bool
}

func (p *printer) write(s string) {
	p.buf.WriteString(s)
	p.atStart = false
}

func (p *printer) line() {
	p.buf.WriteByte('\n')
	p.atStart = true
}

func (p *printer) blank() {
	p.trimRight()
	if p.buf.Len() == 0 {
		return
	}
	b := p.buf.Bytes()
	if len(b) >= 2 && b[len(b)-1] == '\n' && b[len(b)-2] == '\n' {
		return
	}
	p.buf.WriteByte('\n')
	p.atStart = true
}

func (p *printer) trimRight() {
	b := p.buf.Bytes()
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	p.buf.Reset()
	p.buf.Write(b)
}

func (p *printer) trim() {
	b := p.buf.Bytes()
	for len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	p.buf.Reset()
	p.buf.Write(b)
	if len(b) > 0 {
		p.buf.WriteByte('\n')
	}
}

func (p *printer) ind() {
	for i := 0; i < p.indent; i++ {
		p.buf.WriteString("    ")
	}
}

func (p *printer) flush(offset int) {
	for p.ci < len(p.comments) && p.comments[p.ci].Pos().Offset < offset {
		c := p.comments[p.ci]
		p.ci++
		if p.lastLine > 0 && c.Pos().Line > p.lastLine+1 {
			p.blank()
		}
		p.ind()
		p.write(c.Text)
		p.line()
		p.lastLine = c.EndPos().Line
	}
}

func (p *printer) gap(line int) {
	if p.lastLine > 0 && line > p.lastLine+1 {
		p.blank()
	}
}

func (p *printer) use(u *ast.Use) {
	p.flush(u.Pos().Offset)
	p.write("use ")
	p.write(quotePath(u))
	if u.Alias != "" && u.Alias != defaultAlias(u.Path) {
		p.write(" as ")
		p.write(u.Alias)
	}
	p.write(";")
	p.line()
	p.lastLine = u.EndPos().Line
}

func quotePath(u *ast.Use) string {
	if u.Local || strings.Contains(u.Path, "/") || strings.Contains(u.Path, ".") {
		return strconv.Quote(u.Path)
	}
	return u.Path
}

func defaultAlias(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	path = strings.ReplaceAll(path, "-", "_")
	if path == "v2" || path == "v3" || path == "v4" {
		return path
	}
	return path
}

func (p *printer) decl(d ast.Decl) {
	p.flush(d.Pos().Offset)
	p.gap(d.Pos().Line)
	switch d := d.(type) {
	case *ast.FuncDecl:
		p.funcDecl(d)
	case *ast.TypeDecl:
		p.typeDecl(d)
	case *ast.VarDecl:
		p.varDecl(d)
	case *ast.TestDecl:
		p.testDecl(d)
	}
	p.lastLine = d.EndPos().Line
}

func (p *printer) funcDecl(f *ast.FuncDecl) {
	p.ind()
	p.write("fn ")
	p.write(f.Name)
	p.typeParams(f.TypeParams)
	p.params(f.Params)
	p.results(f.Results, false)
	if f.Err {
		p.write(" or error")
	}
	p.write(" = ")
	p.block(f.Body)
	p.write(";")
	p.line()
}

func (p *printer) results(ts []ast.Type, paren bool) {
	if len(ts) == 0 {
		return
	}
	p.write(" ")
	if len(ts) > 1 && paren {
		p.write("(")
	}
	for i, t := range ts {
		if i > 0 {
			p.write(", ")
		}
		p.typ(t)
	}
	if len(ts) > 1 && paren {
		p.write(")")
	}
}

func (p *printer) typeParams(ps []*ast.TypeParam) {
	if len(ps) == 0 {
		return
	}
	p.write("[")
	for i, t := range ps {
		if i > 0 {
			p.write(", ")
		}
		p.write(t.Name)
	}
	p.write("]")
}

func (p *printer) params(ps []*ast.Param) {
	p.write("(")
	for i, x := range ps {
		if i > 0 {
			p.write(", ")
		}
		p.write(x.Name)
		if x.Type != nil {
			p.write(": ")
			p.typ(x.Type)
		}
	}
	p.write(")")
}

func (p *printer) typeDecl(t *ast.TypeDecl) {
	p.ind()
	p.write("type ")
	p.write(t.Name)
	p.typeParams(t.TypeParams)
	p.write(" = ")
	switch t.Kind {
	case ast.TypeStruct:
		p.write("struct {")
		p.line()
		p.lastLine = t.Pos().Line
		p.indent++
		for _, f := range t.Fields {
			p.flush(f.Pos().Offset)
			p.gap(f.Pos().Line)
			p.ind()
			p.write(f.Name)
			p.write(": ")
			p.typ(f.Type)
			p.write(",")
			p.line()
			p.lastLine = f.EndPos().Line
		}
		if len(t.Fields) > 0 && len(t.Methods) > 0 {
			p.blank()
		}
		for i, m := range t.Methods {
			if i > 0 {
				p.blank()
			}
			p.flush(m.Pos().Offset)
			p.funcDecl(m)
		}
		p.indent--
		p.ind()
		p.write("};")
	case ast.TypeEnum:
		p.write("enum {")
		p.line()
		p.lastLine = t.Pos().Line
		p.indent++
		for _, v := range t.Variants {
			p.flush(v.Pos().Offset)
			p.gap(v.Pos().Line)
			p.ind()
			p.write(v.Name)
			if len(v.Fields) > 0 {
				p.write(" { ")
				for i, f := range v.Fields {
					if i > 0 {
						p.write(", ")
					}
					p.write(f.Name)
					p.write(": ")
					p.typ(f.Type)
				}
				p.write(" }")
			}
			p.write(",")
			p.line()
			p.lastLine = v.EndPos().Line
		}
		p.indent--
		p.ind()
		p.write("};")
	case ast.TypeInterface:
		p.write("interface {")
		p.line()
		p.lastLine = t.Pos().Line
		p.indent++
		for _, m := range t.IFace {
			p.flush(m.Pos().Offset)
			p.gap(m.Pos().Line)
			p.ind()
			p.ifaceMethod(m)
			p.line()
			p.lastLine = m.EndPos().Line
		}
		p.indent--
		p.ind()
		p.write("};")
	default:
		p.typ(t.Target)
		p.write(";")
	}
	p.line()
}

func (p *printer) ifaceMethod(m *ast.IFaceMethod) {
	p.write("fn ")
	p.write(m.Name)
	p.params(m.Params)
	p.results(m.Results, false)
	if m.Err {
		p.write(" or error")
	}
	p.write(";")
}

func (p *printer) varDecl(v *ast.VarDecl) {
	p.ind()
	if v.Const {
		p.write("const ")
	} else {
		p.write("var ")
	}
	p.write(strings.Join(v.Names, ", "))
	p.write(" = ")
	p.exprList(v.Values)
	p.write(";")
	p.line()
}

func (p *printer) testDecl(t *ast.TestDecl) {
	p.ind()
	p.write("test ")
	p.write(strconv.Quote(t.Name))
	p.write(" ")
	p.block(t.Body)
	p.write(";")
	p.line()
}

func (p *printer) block(b *ast.Block) {
	p.write("{")
	if b == nil || len(b.Stmts) == 0 {
		p.write("}")
		return
	}
	p.line()
	p.lastLine = b.Pos().Line
	p.indent++
	for _, s := range b.Stmts {
		p.flush(s.Pos().Offset)
		p.gap(s.Pos().Line)
		p.stmt(s)
	}
	p.indent--
	p.ind()
	p.write("}")
}

func (p *printer) stmt(s ast.Stmt) {
	p.ind()
	p.inlineStmt(s)
	p.line()
	p.lastLine = s.EndPos().Line
}

func (p *printer) inlineStmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.Block:
		p.block(s)
	case *ast.EmptyStmt:
		p.write(";")
	case *ast.DeclStmt:
		p.inlineVarDecl(s.Decl)
	case *ast.AssignStmt:
		p.exprList(s.Lhs)
		p.write(" ")
		p.write(s.Op.String())
		p.write(" ")
		p.exprList(s.Rhs)
		p.write(";")
	case *ast.ExprStmt:
		p.expr(s.X)
		p.write(";")
	case *ast.IfStmt:
		p.write("if (")
		p.expr(s.Cond)
		p.write(") ")
		p.block(s.Then)
		if s.Else != nil {
			p.write(" else ")
			p.elsePart(s.Else)
		}
		p.write(";")
	case *ast.ForStmt:
		p.forStmt(s)
	case *ast.ReturnStmt:
		p.write("return")
		if len(s.Results) > 0 {
			p.write(" ")
			p.exprList(s.Results)
		}
		p.write(";")
	case *ast.BreakStmt:
		p.write("break;")
	case *ast.ContinueStmt:
		p.write("continue;")
	case *ast.DeferStmt:
		p.write("defer ")
		p.expr(s.Call)
		p.write(";")
	case *ast.ParStmt:
		p.write("par ")
		p.block(s.Body)
		p.write(";")
	case *ast.WithinStmt:
		p.write("within ")
		p.expr(s.Duration)
		p.write(" ")
		p.block(s.Body)
		p.write(";")
	default:
		p.write(";")
	}
}

func (p *printer) elsePart(s ast.Stmt) {
	if b, ok := s.(*ast.Block); ok {
		p.block(b)
		return
	}
	p.inlineStmt(s)
}

func (p *printer) forStmt(f *ast.ForStmt) {
	if f.Par {
		p.write("par ")
	}
	p.write("for (")
	if f.Range != nil {
		p.write("var ")
		p.write(strings.Join(f.Range.Names, ", "))
		p.write(" in ")
		p.expr(f.Range.X)
	} else {
		if f.Init != nil {
			p.inlineStmt(f.Init)
			p.trimSemi()
		}
		p.write("; ")
		if f.Cond != nil {
			p.expr(f.Cond)
		}
		p.write("; ")
		if f.Post != nil {
			p.inlineStmt(f.Post)
			p.trimSemi()
		}
	}
	p.write(")")
	if f.Limit != nil {
		p.write(" limit ")
		p.expr(f.Limit)
	}
	p.write(" ")
	p.block(f.Body)
	p.write(";")
}

func (p *printer) trimSemi() {
	b := p.buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == ';' {
		b = b[:len(b)-1]
		p.buf.Reset()
		p.buf.Write(b)
	}
}

func (p *printer) inlineVarDecl(v *ast.VarDecl) {
	if v.Const {
		p.write("const ")
	} else {
		p.write("var ")
	}
	p.write(strings.Join(v.Names, ", "))
	p.write(" = ")
	p.exprList(v.Values)
	p.write(";")
}

func (p *printer) exprList(es []ast.Expr) {
	for i, e := range es {
		if i > 0 {
			p.write(", ")
		}
		p.expr(e)
	}
}

const (
	precOr = iota + 1
	precAnd
	precCmp
	precAdd
	precMul
	precUnary
	precPostfix
)

func (p *printer) expr(e ast.Expr) {
	p.exprPrec(e, 0)
}

func (p *printer) exprPrec(e ast.Expr, parent int) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *ast.Ident:
		p.write(e.Name)
	case *ast.BasicLit:
		p.write(e.Value)
	case *ast.SelectorExpr:
		p.exprPrec(e.X, precPostfix)
		p.write(".")
		p.write(e.Name)
	case *ast.CallExpr:
		p.exprPrec(e.Fun, precPostfix)
		if len(e.TypeArgs) > 0 {
			p.write("[")
			for i, t := range e.TypeArgs {
				if i > 0 {
					p.write(", ")
				}
				p.typ(t)
			}
			p.write("]")
		}
		p.write("(")
		p.exprList(e.Args)
		if e.Ellipsis {
			p.write("...")
		}
		p.write(")")
	case *ast.IndexExpr:
		p.exprPrec(e.X, precPostfix)
		p.write("[")
		p.expr(e.Index)
		p.write("]")
	case *ast.SliceExpr:
		p.exprPrec(e.X, precPostfix)
		p.write("[")
		if e.Low != nil {
			p.expr(e.Low)
		}
		p.write(":")
		if e.High != nil {
			p.expr(e.High)
		}
		p.write("]")
	case *ast.UnaryExpr:
		p.paren(e, parent, precUnary)
		p.write(e.Op.String())
		p.exprPrec(e.X, precUnary)
		p.parenClose(e, parent, precUnary)
	case *ast.BinaryExpr:
		prec := binPrec(e.Op)
		p.paren(e, parent, prec)
		p.exprPrec(e.X, prec)
		p.write(" ")
		p.write(e.Op.String())
		p.write(" ")
		p.exprPrec(e.Y, prec+1)
		p.parenClose(e, parent, prec)
	case *ast.ParenExpr:
		p.write("(")
		p.expr(e.X)
		p.write(")")
	case *ast.CompositeLit:
		p.typ(e.Type)
		p.write(" {")
		if len(e.Entries) > 0 {
			p.write(" ")
			for i, en := range e.Entries {
				if i > 0 {
					p.write(", ")
				}
				if en.Key != "" {
					p.write(en.Key)
					p.write(" = ")
				} else if en.KeyExpr != nil {
					p.expr(en.KeyExpr)
					p.write(" = ")
				}
				p.expr(en.Value)
			}
			p.write(" ")
		}
		p.write("}")
	case *ast.FuncLit:
		p.write("fn")
		p.params(e.Params)
		p.results(e.Results, false)
		if e.Err {
			p.write(" or error")
		}
		if e.Arrow != nil {
			p.write(" => ")
			p.expr(e.Arrow)
		} else {
			p.write(" ")
			p.block(e.Body)
		}
	case *ast.NilLit:
		p.write("nil")
		if e.Type != nil {
			p.write("[")
			p.typ(e.Type)
			p.write("]")
		}
	case *ast.MatchExpr:
		p.match(e)
	case *ast.OrExpr:
		p.exprPrec(e.X, precPostfix)
		p.write(" or ")
		switch e.Mode {
		case ast.OrReturn:
			p.write("return")
		case ast.OrPanic:
			p.write("panic(")
			if e.PanicArg != nil {
				p.expr(e.PanicArg)
			}
			p.write(")")
		case ast.OrHandle:
			p.write(e.Name)
			p.write(" ")
			p.block(e.Block)
		}
	default:
		p.write("?")
	}
}

func (p *printer) paren(e ast.Expr, parent, prec int) {
	if prec < parent {
		p.write("(")
	}
}

func (p *printer) parenClose(e ast.Expr, parent, prec int) {
	if prec < parent {
		p.write(")")
	}
}

func binPrec(op token.Kind) int {
	switch op {
	case token.OrOr:
		return precOr
	case token.AndAnd:
		return precAnd
	case token.Eq, token.Ne, token.Lt, token.Le, token.Gt, token.Ge:
		return precCmp
	case token.Plus, token.Minus, token.Pipe, token.Caret:
		return precAdd
	case token.Star, token.Slash, token.Percent, token.Amp, token.Shl, token.Shr:
		return precMul
	}
	return precAdd
}

func (p *printer) match(m *ast.MatchExpr) {
	p.write("match (")
	p.expr(m.Subject)
	p.write(") {")
	if len(m.Cases) == 0 {
		p.write("}")
		return
	}
	p.line()
	p.lastLine = m.Pos().Line
	p.indent++
	for _, c := range m.Cases {
		p.flush(c.Pos().Offset)
		p.ind()
		if c.Default {
			p.write("else => ")
		} else {
			p.write("case ")
			p.pattern(c.Pat)
			p.write(" => ")
		}
		if c.Expr != nil {
			p.expr(c.Expr)
			p.write(";")
			p.line()
		} else if c.Block != nil {
			p.block(c.Block)
			p.write(";")
			p.line()
		} else {
			b := &ast.Block{Stmts: c.Body}
			p.block(b)
			p.write(";")
			p.line()
		}
		p.lastLine = c.EndPos().Line
	}
	p.indent--
	p.ind()
	p.write("}")
}

func (p *printer) pattern(pat *ast.Pattern) {
	if pat == nil {
		p.write("_")
		return
	}
	if pat.Wildcard {
		p.write("_")
		return
	}
	if pat.Literal != nil {
		p.expr(pat.Literal)
		return
	}
	if pat.TypePat != nil {
		p.typ(pat.TypePat)
	} else {
		if pat.Qualifier != "" {
			p.write(pat.Qualifier)
			p.write(".")
		}
		p.write(pat.Name)
	}
	if pat.Bind != "" {
		p.write(" as ")
		p.write(pat.Bind)
	}
	if len(pat.Fields) > 0 {
		p.write(" { ")
		p.write(strings.Join(pat.Fields, ", "))
		p.write(" }")
	}
}

func (p *printer) typ(t ast.Type) {
	p.typPrec(t, 0)
}

func (p *printer) typPrec(t ast.Type, parent int) {
	if t == nil {
		return
	}
	switch t := t.(type) {
	case *ast.NamedType:
		if t.Package != "" {
			p.write(t.Package)
			p.write(".")
		}
		p.write(t.Name)
	case *ast.SliceType:
		p.typeElem(t.Elem)
		p.write("[]")
	case *ast.ArrayType:
		p.typeElem(t.Elem)
		p.write("[")
		p.expr(t.Len)
		p.write("]")
	case *ast.GenericType:
		p.typeElem(t.Base)
		p.write("[")
		for i, a := range t.Args {
			if i > 0 {
				p.write(", ")
			}
			p.typ(a)
		}
		p.write("]")
	case *ast.MapType:
		p.write("map[")
		p.typ(t.Key)
		p.write(", ")
		p.typ(t.Value)
		p.write("]")
	case *ast.PointerType:
		p.write("*")
		p.typPrec(t.Elem, precUnary)
	case *ast.FuncType:
		p.parenType(t, parent, precUnary)
		p.write("fn(")
		for i, a := range t.Params {
			if i > 0 {
				p.write(", ")
			}
			p.typ(a)
		}
		p.write(")")
		p.results(t.Results, true)
		if t.Err {
			p.write(" or error")
		}
		p.parenTypeClose(t, parent, precUnary)
	case *ast.InterfaceType:
		p.write("interface {")
		if len(t.Methods) > 0 {
			p.line()
			p.indent++
			for _, m := range t.Methods {
				p.ind()
				p.ifaceMethod(m)
				p.line()
			}
			p.indent--
			p.ind()
		}
		p.write("}")
	case *ast.StructType:
		p.write("struct {")
		for i, f := range t.Fields {
			if i == 0 {
				p.line()
				p.indent++
			}
			p.ind()
			p.write(f.Name)
			p.write(": ")
			p.typ(f.Type)
			p.write(",")
			p.line()
		}
		if len(t.Fields) > 0 {
			p.indent--
			p.ind()
		}
		p.write("}")
	case *ast.GoType:
		p.write(t.Text)
	}
}

func (p *printer) typeElem(t ast.Type) {
	need := false
	switch t.(type) {
	case *ast.PointerType, *ast.FuncType:
		need = true
	}
	if need {
		p.write("(")
	}
	p.typ(t)
	if need {
		p.write(")")
	}
}

func (p *printer) parenType(t ast.Type, parent, prec int) {
	if prec < parent {
		p.write("(")
	}
}

func (p *printer) parenTypeClose(t ast.Type, parent, prec int) {
	if prec < parent {
		p.write(")")
	}
}
