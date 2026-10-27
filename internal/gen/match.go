package gen

import (
	"sort"

	"github.com/bongdo/strizh/internal/ast"
)

func (w *writer) matchExpr(m *ast.MatchExpr) string {
	sub := w.sub()
	sub.indent = 1
	rt := "any"
	if m.ResultType != nil {
		rt = w.typeStr(m.ResultType)
	}
	sub.matchSwitch(m, true)
	sub.line(`panic("unreachable")`)
	return "func() " + rt + " {\n" + sub.buf.String() + "}()"
}

func (w *writer) matchSwitch(m *ast.MatchExpr, exprMode bool) {
	if m.TypeSwitch {
		w.matchTypeSwitch(m, exprMode)
		return
	}
	enum, qual := w.matchEnum(m)
	subj := w.expr(m.Subject)
	if enum != nil {
		bind := false
		for _, cs := range m.Cases {
			if cs.Pat != nil && len(cs.Pat.Fields) > 0 {
				bind = true
			}
		}
		tv := ""
		if bind {
			tv = w.next("m")
			w.linef("switch %s := (%s).(type) {", tv, subj)
		} else {
			w.linef("switch (%s).(type) {", subj)
		}
		for _, cs := range m.Cases {
			w.matchEnumCase(cs, tv, enum, qual, exprMode)
		}
		w.line("}")
		return
	}
	w.linef("switch %s {", subj)
	for _, cs := range m.Cases {
		w.matchValueCase(cs, exprMode)
	}
	w.line("}")
}

func (w *writer) matchTypeSwitch(m *ast.MatchExpr, exprMode bool) {
	subj := w.expr(m.Subject)
	bind := false
	for _, cs := range m.Cases {
		if cs.Pat != nil && cs.Pat.Bind != "" && cs.Pat.Bind != "_" {
			bind = true
			break
		}
	}
	if bind {
		w.linef("switch _s_m := (%s).(type) {", subj)
	} else {
		w.linef("switch (%s).(type) {", subj)
	}
	for _, cs := range m.Cases {
		pat := cs.Pat
		if cs.Default || pat == nil || pat.Wildcard {
			w.line("default:")
			w.indent++
			w.caseBody(cs, exprMode)
			w.indent--
			continue
		}
		w.line("case " + w.typeStr(pat.TypePat) + ":")
		w.indent++
		if pat.Bind != "" && pat.Bind != "_" {
			w.linef("%s := _s_m", pat.Bind)
			w.linef("_ = %s", pat.Bind)
		}
		w.caseBody(cs, exprMode)
		w.indent--
	}
	w.line("}")
}

func (w *writer) matchEnum(m *ast.MatchExpr) (*ast.TypeDecl, string) {
	if n, ok := m.SubjectType.(*ast.NamedType); ok && n.Decl != nil && n.Decl.Kind == ast.TypeEnum {
		qual := n.PkgAlias
		if qual != "" {
			w.f.usePkg(qual)
		} else {
			for _, cs := range m.Cases {
				if cs.Pat != nil && cs.Pat.Qualifier != "" {
					qual = w.validQualifier(cs.Pat.Qualifier)
					if qual != "" {
						break
					}
				}
			}
		}
		if qual == "" {
			qual = w.enumImportQualifier(n.Decl)
		}
		return n.Decl, qual
	}
	for _, cs := range m.Cases {
		if cs.Pat != nil && cs.Pat.Variant != nil {
			v := cs.Pat.Variant
			if v.Enum != nil {
				qual := w.validQualifier(cs.Pat.Qualifier)
				if qual == "" {
					qual = w.enumImportQualifier(v.Enum)
				}
				return v.Enum, qual
			}
		}
	}
	return nil, ""
}

func (w *writer) enumImportQualifier(d *ast.TypeDecl) string {
	if d == nil {
		return ""
	}
	var aliases []string
	for alias := range w.f.pkg.Imports {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		imp := w.f.pkg.Imports[alias]
		if imp.Local != nil && imp.Local.Types[d.Name] == d {
			w.f.usePkg(alias)
			return alias
		}
	}
	return ""
}

func (w *writer) validQualifier(q string) string {
	if q == "" {
		return ""
	}
	if w.f.pkg.Imports[q] != nil {
		w.f.usePkg(q)
		return q
	}
	return ""
}

func (w *writer) matchEnumCase(cs *ast.Case, tv string, enum *ast.TypeDecl, qual string, exprMode bool) {
	pat := cs.Pat
	v := (*ast.Variant)(nil)
	if pat == nil || pat.Wildcard {
		w.line("default:")
		w.indent++
		w.caseBody(cs, exprMode)
		w.indent--
		return
	}
	v = pat.Variant
	if v == nil {
		v = variantByName(enum, pat.Name)
	}
	if v == nil {
		w.line("default:")
		w.indent++
		w.caseBody(cs, exprMode)
		w.indent--
		return
	}
	caseQual := qual
	if pat.Qualifier != "" {
		caseQual = w.validQualifier(pat.Qualifier)
		if caseQual == "" {
			caseQual = qual
		}
	}
	w.line("case *" + qualified(caseQual, w.f.g.variantName(v)) + ":")
	w.indent++
	if tv != "" {
		for i, name := range pat.Fields {
			if name == "_" {
				continue
			}
			field := patternField(pat, i, v)
			if field == "" {
				continue
			}
			w.linef("%s := %s.%s", name, tv, field)
		}
		for _, name := range pat.Fields {
			if name != "_" {
				w.linef("_ = %s", name)
			}
		}
	}
	w.caseBody(cs, exprMode)
	w.indent--
}

func (w *writer) matchValueCase(cs *ast.Case, exprMode bool) {
	pat := cs.Pat
	if pat == nil || pat.Wildcard {
		w.line("default:")
		w.indent++
		w.caseBody(cs, exprMode)
		w.indent--
		return
	}
	lit := ""
	if pat.Literal != nil {
		lit = w.expr(pat.Literal)
	} else if pat.Name != "" {
		lit = pat.Name
	}
	if lit == "" {
		w.line("default:")
	} else {
		w.line("case " + lit + ":")
	}
	w.indent++
	w.caseBody(cs, exprMode)
	w.indent--
}

func (w *writer) caseBody(cs *ast.Case, exprMode bool) {
	if exprMode {
		if cs.Expr != nil {
			w.directive(cs.Expr)
			w.line("return " + w.expr(cs.Expr))
			return
		}
		w.line(`panic("unreachable")`)
		return
	}
	var stmts []ast.Stmt
	if cs.Block != nil {
		stmts = cs.Block.Stmts
	} else {
		stmts = cs.Body
	}
	if len(stmts) == 0 {
		if cs.Expr != nil {
			w.directive(cs.Expr)
			w.line(w.expr(cs.Expr))
		}
		return
	}
	block := cs.Block != nil
	if block {
		w.line("{")
		w.indent++
	}
	mark := ""
	if w.deferStack && hasDirectDefer(stmts) {
		mark = w.next("mark")
		w.linef("%s := _s_d.Mark()", mark)
	}
	w.genStmts(stmts)
	if mark != "" {
		w.linef("_s_d.RunTo(%s)", mark)
	}
	if block {
		w.indent--
		w.line("}")
	}
}

func variantByName(d *ast.TypeDecl, name string) *ast.Variant {
	if d == nil {
		return nil
	}
	for _, v := range d.Variants {
		if v.Name == name {
			return v
		}
	}
	return nil
}

func patternField(pat *ast.Pattern, i int, v *ast.Variant) string {
	if i < len(pat.FieldDecls) && pat.FieldDecls[i] != nil {
		return pat.FieldDecls[i].Name
	}
	if i < len(pat.Fields) && v != nil {
		name := pat.Fields[i]
		for _, fld := range v.Fields {
			if fld.Name == name {
				return fld.Name
			}
		}
	}
	if i < len(pat.Fields) {
		return pat.Fields[i]
	}
	return ""
}
