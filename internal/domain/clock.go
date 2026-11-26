package domain

import (
	"time"

	"github.com/google/uuid"
)

//go:generate mockgen -destination=../testkit/mocks/domain/mock_clock.go -package=domainmock unicorn/internal/domain Clock,IDGenerator

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type IDGenerator interface {
	NewID() (uuid.UUID, error)
}

type UUIDv7Generator struct{}

func (UUIDv7Generator) NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}
