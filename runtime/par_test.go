package strizhrt

import (
	"errors"
	"testing"
)

func TestParErrorsLowestIndexWins(t *testing.T) {
	var p ParErrors
	p.Add(2, errors.New("two"))
	p.Add(0, errors.New("zero"))
	p.Add(1, errors.New("one"))
	got := p.First()
	if got == nil || got.Error() != "zero" {
		t.Fatalf("First() = %v, want zero", got)
	}
}

func TestParErrorsUnorderedFirstCompleted(t *testing.T) {
	var p ParErrors
	p.Add(-1, errors.New("first"))
	p.Add(-1, errors.New("second"))
	got := p.First()
	if got == nil || got.Error() != "first" {
		t.Fatalf("First() = %v, want first", got)
	}
}

func TestParErrorsIndexedBeatsUnordered(t *testing.T) {
	var p ParErrors
	p.Add(-1, errors.New("map"))
	p.Add(3, errors.New("indexed"))
	got := p.First()
	if got == nil || got.Error() != "indexed" {
		t.Fatalf("First() = %v, want indexed", got)
	}
}

func TestParErrorsEmpty(t *testing.T) {
	var p ParErrors
	if got := p.First(); got != nil {
		t.Fatalf("First() = %v, want nil", got)
	}
}
