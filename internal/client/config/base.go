package config

import "sync"

type BaseConfigManager[T any] struct {
	mu   sync.RWMutex
	conf *T

	mgr *Manager
}

// Return the read-only configuration by value
func (a *BaseConfigManager[T]) C() T {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return *a.conf
}

type ConfigModifierFunc[T any] func(c *T)

func (a *BaseConfigManager[T]) Set(setFunc ConfigModifierFunc[T]) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// call the set function
	setFunc(a.conf)
}

func (a *BaseConfigManager[T]) Save() {
	// save the main config, dont lock, the manager will lock us
	a.mgr.Save()
}

func (a *BaseConfigManager[T]) lock() {
	a.mu.Lock()
}

func (a *BaseConfigManager[T]) unlock() {
	a.mu.Unlock()
}
