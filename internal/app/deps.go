package app

import "unicorn/internal/domain"

type Deps struct {
	Clock       domain.Clock
	IDGenerator domain.IDGenerator
}

type Option func(*Deps)

func WithClock(clock domain.Clock) Option {
	return func(deps *Deps) {
		deps.Clock = clock
	}
}

func WithIDGenerator(idGen domain.IDGenerator) Option {
	return func(deps *Deps) {
		deps.IDGenerator = idGen
	}
}

func resolveDeps(opts []Option) Deps {
	deps := Deps{
		Clock:       domain.SystemClock{},
		IDGenerator: domain.UUIDv7Generator{},
	}
	for _, opt := range opts {
		opt(&deps)
	}
	return deps
}
