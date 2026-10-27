package strizhrt

import (
	"fmt"
	"runtime/debug"
	"sync"
)

type DeferStack struct {
	fns []func()
}

func (d *DeferStack) Mark() int {
	return len(d.fns)
}

func (d *DeferStack) Push(fn func()) {
	d.fns = append(d.fns, fn)
}

func (d *DeferStack) RunTo(mark int) {
	if mark < 0 {
		mark = 0
	}
	if mark > len(d.fns) {
		mark = len(d.fns)
	}
	pending := d.fns[mark:]
	d.fns = d.fns[:mark]
	for i := len(pending) - 1; i >= 0; i-- {
		pending[i]()
	}
}

func Assert(ok bool, pos string) {
	if !ok {
		panic("assert failed at " + pos)
	}
}

func AssertEq[T comparable](got, want T, pos string) {
	if got != want {
		panic(fmt.Sprintf("assert_eq failed at %s: got %v, want %v", pos, got, want))
	}
}

func Has[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

type ParError struct {
	Index int
	Err   error
}

type PanicError struct {
	Value any
	Stack []byte
}

func NewPanicError(v any) *PanicError {
	return &PanicError{Value: v, Stack: debug.Stack()}
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v\n%s", e.Value, e.Stack)
}

func AsError(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return fmt.Errorf("%v", v)
}

type ParErrors struct {
	mu   sync.Mutex
	errs []ParError
}

func (p *ParErrors) Add(index int, err error) {
	if err == nil {
		return
	}
	p.mu.Lock()
	p.errs = append(p.errs, ParError{Index: index, Err: err})
	p.mu.Unlock()
}

func (p *ParErrors) First() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.errs) == 0 {
		return nil
	}
	best := -1
	for i, e := range p.errs {
		if e.Index < 0 {
			continue
		}
		if best < 0 || e.Index < p.errs[best].Index {
			best = i
		}
	}
	if best >= 0 {
		return p.errs[best].Err
	}
	return p.errs[0].Err
}
