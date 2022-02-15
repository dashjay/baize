package caches

import (
	"sync"
)

type Metrics struct {
	sync.RWMutex
	hit   int64
	miss  int64
	Total int64
}

func (m *Metrics) Hit() {
	m.Lock()
	defer m.Unlock()
	m.hit++
	m.Total++
}

func (m *Metrics) GetHit() int64 {
	m.RLock()
	defer m.RUnlock()
	return m.hit
}

func (m *Metrics) Miss() {
	m.Lock()
	defer m.Unlock()
	m.miss++
	m.Total++
}

func (m *Metrics) GetMiss() int64 {
	m.RLock()
	defer m.RUnlock()
	return m.miss
}

func (m *Metrics) GetTotal() int64 {
	m.RLock()
	defer m.RUnlock()
	return m.Total
}

func (m *Metrics) GetHitRate() float64 {
	return float64(m.GetHit()) / float64(m.GetTotal()) * 100
}
