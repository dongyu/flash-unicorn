package testkit

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type FakeClock struct {
	mu  sync.RWMutex
	now time.Time
}

func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t.UTC()}
}

func (c *FakeClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t.UTC()
}

func (c *FakeClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

type SeqIDGenerator struct {
	counter atomic.Uint64
}

func (g *SeqIDGenerator) NewID() (uuid.UUID, error) {
	var id uuid.UUID
	binary.BigEndian.PutUint64(id[8:], g.counter.Add(1))
	id[6] = 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}
