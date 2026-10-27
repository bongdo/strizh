package sema

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/bongdo/strizh/internal/ast"
)

type sign struct {
	params   []string
	results  []string
	variadic bool
	unknown  bool
}

type ifaceMethodSig struct {
	name string
	sign sign
}

type ifaceTarget struct {
	name    string
	pkg     *Package
	methods []ifaceMethodSig
}

func unwrapAliasType(t ast.Type) ast.Type {
	for i := 0; i < 100; i++ {
		n, ok := t.(*ast.NamedType)
		if !ok || n.Decl == nil || n.Decl.Kind != ast.TypeAlias {
			return t
		}
		t = n.Decl.Target
	}
	return t
}

func (p *Program) typeKey(t ast.Type) (string, bool) {
	t = unwrapAliasType(t)
	switch t := t.(type) {
	case *ast.NamedType:
		if t.Variant != nil {
			enum := t.Variant.Enum
			if enum == nil {
				return "", false
			}
			name := t.Variant.GoName
			if name == "" {
				name = enum.GoName + t.Variant.Name
			}
			path, ok := p.packagePath(enum)
			if !ok {
				return "", false
			}
			return path + "." + name, true
		}
		if t.Decl != nil {
			if len(t.Decl.TypeParams) > 0 {
				return "", false
			}
			name := t.Decl.GoName
			if name == "" {
				name = t.Decl.Name
			}
			path, ok := p.packagePath(t.Decl)
			if !ok {
				return "", false
			}
			return path + "." + name, true
		}
		if gt := p.goTypeOf(t); gt != nil {
			return goKey(gt)
		}
		if t.Package == "" {
			k, ok := builtinTypes[t.Name]
			if !ok || k == "" {
				return "", false
			}
			switch k {
			case "rune":
				k = "int32"
			case "byte":
				k = "uint8"
			}
			return k, true
		}
		return "", false
	case *ast.PointerType:
		k, ok := p.typeKey(t.Elem)
		if !ok {
			return "", false
		}
		return "*" + k, true
	case *ast.SliceType:
		k, ok := p.typeKey(t.Elem)
		if !ok {
			return "", false
		}
		return "[]" + k, true
	case *ast.MapType:
		key, ok := p.typeKey(t.Key)
		if !ok {
			return "", false
		}
		val, ok := p.typeKey(t.Value)
		if !ok {
			return "", false
		}
		return "map[" + key + "]" + val, true
	case *ast.FuncType:
		s := p.signOf(t.Params, t.Results, t.Err)
		if s.unknown {
			return "", false
		}
		params := s.params
		if t.NeedsCtx {
			params = append([]string{"context.Context"}, params...)
		}
		return "func(" + strings.Join(params, ", ") + ")(" + strings.Join(s.results, ", ") + ")", true
	case *ast.GoType:
		if gt := p.goOf[t]; gt != nil {
			return goKey(gt)
		}
		return "", false
	}
	return "", false
}

func (p *Program) packagePath(d *ast.TypeDecl) (string, bool) {
	pkg := p.declPkg[d]
	if pkg == nil || p.Module == "" {
		return "", false
	}
	if pkg.Rel == "." {
		return p.Module, true
	}
	return p.Module + "/" + pkg.Rel, true
}

func (p *Program) signOf(params, results []ast.Type, err bool) sign {
	s := sign{}
	for _, t := range params {
		k, ok := p.typeKey(t)
		if !ok {
			s.unknown = true
			continue
		}
		s.params = append(s.params, k)
	}
	for _, t := range results {
		k, ok := p.typeKey(t)
		if !ok {
			s.unknown = true
			continue
		}
		s.results = append(s.results, k)
	}
	if err {
		s.results = append(s.results, "error")
	}
	return s
}

func (p *Program) funcSign(fd *ast.FuncDecl) sign {
	params := make([]ast.Type, 0, len(fd.Params))
	for _, prm := range fd.Params {
		params = append(params, prm.Type)
	}
	return p.signOf(params, fd.Results, fd.Err)
}

func (p *Program) ifaceSign(m *ast.IFaceMethod) sign {
	params := make([]ast.Type, 0, len(m.Params))
	for _, prm := range m.Params {
		params = append(params, prm.Type)
	}
	return p.signOf(params, m.Results, m.Err)
}

func (p *Program) ifaceTargetOf(t ast.Type) *ifaceTarget {
	if t == nil {
		return nil
	}
	t = unwrapAliasType(t)
	if n, ok := t.(*ast.NamedType); ok && n.Decl == nil && n.Package == "" {
		switch n.Name {
		case "any":
			return nil
		case "error":
			return p.goIfaceTarget(types.Universe.Lookup("error").Type())
		}
	}
	if n, ok := t.(*ast.NamedType); ok && n.Decl != nil && n.Decl.Kind == ast.TypeInterface {
		d := n.Decl
		if len(d.TypeParams) > 0 {
			return nil
		}
		tgt := &ifaceTarget{name: d.Name, pkg: p.declPkg[d], methods: make([]ifaceMethodSig, 0, len(d.IFace))}
		for _, m := range d.IFace {
			tgt.methods = append(tgt.methods, ifaceMethodSig{name: m.Name, sign: p.ifaceSign(m)})
		}
		return tgt
	}
	if gt := p.goTypeOf(t); gt != nil {
		return p.goIfaceTarget(gt)
	}
	return nil
}

func (p *Program) goIfaceTarget(t types.Type) *ifaceTarget {
	u := types.Unalias(t)
	var iface *types.Interface
	name := "interface"
	switch u := u.(type) {
	case *types.Named:
		if u.TypeParams().Len() > 0 || u.TypeArgs().Len() > 0 {
			return nil
		}
		it, ok := u.Underlying().(*types.Interface)
		if !ok {
			return nil
		}
		iface = it
		name = goTypeName(u)
	case *types.Interface:
		iface = u
	default:
		return nil
	}
	iface = iface.Complete()
	tgt := &ifaceTarget{name: name, methods: make([]ifaceMethodSig, 0, iface.NumMethods())}
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			continue
		}
		tgt.methods = append(tgt.methods, ifaceMethodSig{name: m.Name(), sign: goSignOf(sig)})
	}
	return tgt
}

func (p *Program) methodSet(t ast.Type) (map[string]sign, *ast.TypeDecl, bool) {
	if gt := p.goTypeOf(t); gt != nil {
		out, ok := p.goMethodSet(gt)
		return out, nil, ok
	}
	t = unwrapAliasType(t)
	if n, ok := t.(*ast.NamedType); ok && n.Decl == nil && n.Package == "" {
		switch n.Name {
		case "any":
			return map[string]sign{}, nil, true
		case "error":
			out, ok := p.goMethodSet(types.Universe.Lookup("error").Type())
			return out, nil, ok
		}
	}
	switch t := t.(type) {
	case *ast.PointerType:
		n, ok := unwrapAliasType(t.Elem).(*ast.NamedType)
		if !ok || n.Decl == nil || n.Decl.Kind != ast.TypeStruct || len(n.Decl.TypeParams) > 0 {
			return nil, nil, false
		}
		out := make(map[string]sign, len(n.Decl.Methods))
		for _, m := range n.Decl.Methods {
			out[m.Name] = p.funcSign(m)
		}
		return out, n.Decl, true
	case *ast.NamedType:
		d := t.Decl
		if d == nil || len(d.TypeParams) > 0 {
			return nil, nil, false
		}
		switch d.Kind {
		case ast.TypeInterface:
			out := make(map[string]sign, len(d.IFace))
			for _, m := range d.IFace {
				out[m.Name] = p.ifaceSign(m)
			}
			return out, d, true
		case ast.TypeStruct:
			return map[string]sign{}, d, true
		}
	}
	return nil, nil, false
}

func (p *Program) goMethodSet(t types.Type) (map[string]sign, bool) {
	u := types.Unalias(t)
	if n, ok := u.(*types.Named); ok && (n.TypeParams().Len() > 0 || n.TypeArgs().Len() > 0) {
		return nil, false
	}
	set := types.NewMethodSet(u)
	out := make(map[string]sign, set.Len())
	for i := 0; i < set.Len(); i++ {
		sel := set.At(i)
		sig, ok := sel.Obj().Type().(*types.Signature)
		if !ok {
			continue
		}
		out[sel.Obj().Name()] = goSignOf(sig)
	}
	return out, true
}

func goSignOf(sig *types.Signature) sign {
	s := sign{variadic: sig.Variadic()}
	for i := 0; i < sig.Params().Len(); i++ {
		t := sig.Params().At(i).Type()
		if sig.Variadic() && i == sig.Params().Len()-1 {
			if sl, ok := types.Unalias(t).(*types.Slice); ok {
				t = sl.Elem()
			}
		}
		if k, ok := goKey(t); ok {
			s.params = append(s.params, k)
		} else {
			s.unknown = true
		}
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if k, ok := goKey(sig.Results().At(i).Type()); ok {
			s.results = append(s.results, k)
		} else {
			s.unknown = true
		}
	}
	return s
}

func goKey(t types.Type) (string, bool) {
	if t == nil {
		return "", false
	}
	t = types.Unalias(t)
	switch t := t.(type) {
	case *types.Basic:
		if t.Info()&types.IsUntyped != 0 {
			return "", false
		}
		switch t.Kind() {
		case types.Uint8:
			return "uint8", true
		case types.Int32:
			return "int32", true
		}
		return t.Name(), true
	case *types.Named:
		if t.TypeParams().Len() > 0 || t.TypeArgs().Len() > 0 {
			return "", false
		}
		obj := t.Obj()
		if obj.Pkg() == nil {
			return obj.Name(), true
		}
		return obj.Pkg().Path() + "." + obj.Name(), true
	case *types.Pointer:
		k, ok := goKey(t.Elem())
		if !ok {
			return "", false
		}
		return "*" + k, true
	case *types.Slice:
		k, ok := goKey(t.Elem())
		if !ok {
			return "", false
		}
		return "[]" + k, true
	case *types.Array:
		k, ok := goKey(t.Elem())
		if !ok {
			return "", false
		}
		return fmt.Sprintf("[%d]%s", t.Len(), k), true
	case *types.Map:
		key, ok := goKey(t.Key())
		if !ok {
			return "", false
		}
		val, ok := goKey(t.Elem())
		if !ok {
			return "", false
		}
		return "map[" + key + "]" + val, true
	case *types.Signature:
		if t.Variadic() {
			return "", false
		}
		s := goSignOf(t)
		if s.unknown {
			return "", false
		}
		return "func(" + strings.Join(s.params, ", ") + ")(" + strings.Join(s.results, ", ") + ")", true
	case *types.Interface:
		if t.NumMethods() == 0 {
			return "any", true
		}
		return "", false
	}
	return "", false
}

func goTypeName(t types.Type) string {
	return types.TypeString(t, func(pkg *types.Package) string {
		if pkg == nil {
			return ""
		}
		return pkg.Name()
	})
}

func signDiff(want, have sign) string {
	if want.unknown || have.unknown {
		return ""
	}
	n := len(want.params)
	if len(have.params) < n {
		n = len(have.params)
	}
	for i := 0; i < n; i++ {
		if want.params[i] != have.params[i] {
			return fmt.Sprintf("has parameter %d of type %s, want %s", i+1, have.params[i], want.params[i])
		}
	}
	if len(want.params) != len(have.params) {
		return fmt.Sprintf("has %d parameters, want %d", len(have.params), len(want.params))
	}
	if want.variadic != have.variadic {
		return "has a different variadic signature"
	}
	n = len(want.results)
	if len(have.results) < n {
		n = len(have.results)
	}
	for i := 0; i < n; i++ {
		if want.results[i] != have.results[i] {
			return fmt.Sprintf("has result %d of type %s, want %s", i+1, have.results[i], want.results[i])
		}
	}
	if len(want.results) != len(have.results) {
		if len(want.results) > n && want.results[len(want.results)-1] == "error" {
			return "is missing an error result"
		}
		if len(have.results) > n && have.results[len(have.results)-1] == "error" {
			return "has an unexpected error result"
		}
		return fmt.Sprintf("has %d results, want %d", len(have.results), len(want.results))
	}
	return ""
}

func (c *checker) checkAssign(dst ast.Type, e ast.Expr) {
	if dst == nil || e == nil {
		return
	}
	if call, ok := e.(*ast.CallExpr); ok && call.Info != nil && call.Info.Results > 1 {
		return
	}
	src := c.exprType(e)
	if src == nil {
		return
	}
	tgt := c.p.ifaceTargetOf(dst)
	if tgt == nil || len(tgt.methods) == 0 {
		return
	}
	got, decl, known := c.p.methodSet(src)
	if !known {
		return
	}
	var problems []string
	for _, want := range tgt.methods {
		have, ok := got[want.name]
		if !ok {
			problems = append(problems, "missing method "+want.name)
			continue
		}
		if !exported(want.name) && decl != nil && !samePkg(tgt.pkg, c.p.declPkg[decl]) {
			problems = append(problems, "method "+want.name+" is not exported")
			continue
		}
		if diff := signDiff(want.sign, have); diff != "" {
			problems = append(problems, "method "+want.name+" "+diff)
		}
	}
	if len(problems) == 0 {
		return
	}
	msg := fmt.Sprintf("%s does not implement %s: %s", typeDesc(src), tgt.name, strings.Join(problems, "; "))
	if decl != nil && decl.Kind == ast.TypeStruct && !isPointerType(src) {
		msg += " (methods use pointer receivers; pass a pointer)"
	}
	c.p.errorf(c.fc.file.Path, e.Pos(), "%s", msg)
}

func samePkg(a, b *Package) bool {
	return a != nil && a == b
}

func isPointerType(t ast.Type) bool {
	_, ok := unwrapAliasType(t).(*ast.PointerType)
	return ok
}

func typeDesc(t ast.Type) string {
	switch t := t.(type) {
	case *ast.PointerType:
		return "*" + typeDesc(t.Elem)
	case *ast.NamedType:
		if t.Variant != nil {
			if t.Variant.Enum != nil {
				return t.Variant.Enum.Name + "." + t.Variant.Name
			}
			return t.Variant.Name
		}
		name := t.GoName
		if name == "" {
			name = t.Name
		}
		if t.PkgAlias != "" {
			return t.PkgAlias + "." + name
		}
		return name
	case *ast.SliceType:
		return typeDesc(t.Elem) + "[]"
	case *ast.MapType:
		return "map[" + typeDesc(t.Key) + "]" + typeDesc(t.Value)
	case *ast.FuncType:
		return "fn"
	case *ast.GoType:
		return t.Text
	}
	return "value"
}
