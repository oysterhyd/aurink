package docsession

import (
	"log/slog"
	"slices"

	"aurink/internal/contract"
)

type listener[T any] struct{ fn func(T) }

type listeners[T any] struct {
	subs     []*listener[T]
	pending  []T
	emitting bool
}

func (l *listeners[T]) subscribe(fn func(T)) func() {
	s := &listener[T]{fn: fn}
	l.subs = append(l.subs, s)
	return func() {
		if i := slices.Index(l.subs, s); i >= 0 {
			l.subs = slices.Delete(l.subs, i, i+1)
		}
		s.fn = nil
	}
}

func (l *listeners[T]) emit(event T) {
	l.pending = append(l.pending, event)
	if l.emitting {
		return
	}
	l.emitting = true
	defer func() { l.emitting = false }()
	for len(l.pending) > 0 {
		event := l.pending[0]
		l.pending = slices.Delete(l.pending, 0, 1)
		for _, s := range slices.Clone(l.subs) {
			if s.fn != nil {
				func() { defer logPanic(); s.fn(event) }()
			}
		}
	}
}

func logPanic() {
	if p := recover(); p != nil {
		slog.Error("docsession callback panicked", "panic", p)
	}
}

func callDone(done func(error), err error) {
	if done != nil {
		defer logPanic()
		done(err)
	}
}

func callOpen(done func(contract.Session, error), s contract.Session, err error) {
	if done != nil {
		defer logPanic()
		done(s, failureOrNil(err))
	}
}

func failure(err error) *contract.Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*contract.Error); ok {
		return e
	}
	return contract.E(contract.CodeIO, "docsession", err)
}
