package sema

import "github.com/bongdo/strizh/internal/ast"

type ctxEdge struct {
	from ast.Node
	to   ast.Node
	call *ast.CallExpr
}

type litCall struct {
	lit      *ast.FuncLit
	goCall   bool
	expected ast.Type
}

func (p *Program) colorContext() {
	needs := p.needsCtx
	for {
		changed := false
		for _, e := range p.edges {
			if e.from == nil || e.to == nil {
				continue
			}
			if needs[e.to] && !needs[e.from] {
				needs[e.from] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for _, e := range p.edges {
		if e.call != nil && e.to != nil && needs[e.to] {
			if e.call.Info != nil {
				e.call.Info.NeedsCtx = true
			}
		}
	}
	for _, pkg := range p.Packages {
		for _, f := range pkg.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					if needs[n] {
						n.NeedsCtx = true
					}
				case *ast.FuncLit:
					if needs[n] {
						n.NeedsCtx = true
					}
				}
				return true
			})
		}
	}
	for _, lc := range p.litCalls {
		if lc.lit == nil || !needs[lc.lit] {
			continue
		}
		if lc.goCall {
			lc.lit.GoCallback = true
			continue
		}
		if ft := funcTypeOf(lc.expected); ft != nil && !ft.NeedsCtx {
			p.errorf(p.Root, lc.lit.Pos(), "callback needs a context but the target does not pass one")
		}
	}
}

func funcTypeOf(t ast.Type) *ast.FuncType {
	if ft, ok := t.(*ast.FuncType); ok {
		return ft
	}
	if n, ok := t.(*ast.NamedType); ok && n.Decl != nil && n.Decl.Kind == ast.TypeAlias {
		return funcTypeOf(n.Decl.Target)
	}
	return nil
}

func variantByPattern(enum *ast.TypeDecl, pat *ast.Pattern) *ast.Variant {
	if pat == nil {
		return nil
	}
	return findVariant(enum, pat.Name)
}
