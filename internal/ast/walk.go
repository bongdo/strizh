package ast

import (
	"reflect"

	"github.com/bongdo/strizh/internal/token"
)

var skipped = map[reflect.Type]bool{
	reflect.TypeOf((*Ref)(nil)):      true,
	reflect.TypeOf((*CallInfo)(nil)): true,
	reflect.TypeOf(token.Pos{}):      true,
	reflect.TypeOf(Span{}):           true,
}

func Inspect(n Node, f func(Node) bool) {
	visited := make(map[uintptr]bool)
	walkValue(reflect.ValueOf(n), f, visited)
}

func walkValue(v reflect.Value, f func(Node) bool, visited map[uintptr]bool) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		walkValue(v.Elem(), f, visited)
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		if skipped[v.Type()] {
			return
		}
		if n, ok := v.Interface().(Node); ok {
			p := v.Pointer()
			if visited[p] {
				return
			}
			visited[p] = true
			if !f(n) {
				return
			}
		}
		walkValue(v.Elem(), f, visited)
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if t.Field(i).PkgPath != "" {
				continue
			}
			if skipped[t.Field(i).Type] {
				continue
			}
			walkValue(v.Field(i), f, visited)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			walkValue(v.Index(i), f, visited)
		}
	}
}

func NodeAt(root Node, offset int) Node {
	var best Node
	bestWidth := int(^uint(0) >> 1)
	Inspect(root, func(n Node) bool {
		start := n.Pos().Offset
		end := n.EndPos().Offset
		if start <= offset && offset <= end {
			width := end - start
			if width <= bestWidth {
				best = n
				bestWidth = width
			}
		}
		return true
	})
	return best
}
