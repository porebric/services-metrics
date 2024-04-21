package exporter

import (
	"sync/atomic"
)

type TotalMemoryCounter struct {
	value atomic.Uint64
	_     [56]byte
}

func (c *TotalMemoryCounter) Add(value uint64) {
	c.value.Add(value)
}

func (c *TotalMemoryCounter) Clear() {
	c.value.Store(0)
}

func (c *TotalMemoryCounter) Get() uint64 {
	return c.value.Load()
}
