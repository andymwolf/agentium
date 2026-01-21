package agent

import (
	"fmt"
	"sync"
)

var (
	registry     = make(map[string]func() Agent)
	registryLock sync.RWMutex
)

// Register adds an agent factory to the registry
func Register(name string, factory func() Agent) {
	registryLock.Lock()
	defer registryLock.Unlock()
	registry[name] = factory
}

// Get retrieves an agent by name from the registry
func Get(name string) (Agent, error) {
	registryLock.RLock()
	defer registryLock.RUnlock()

	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", name)
	}

	return factory(), nil
}

// List returns all registered agent names
func List() []string {
	registryLock.RLock()
	defer registryLock.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Exists checks if an agent is registered
func Exists(name string) bool {
	registryLock.RLock()
	defer registryLock.RUnlock()
	_, ok := registry[name]
	return ok
}
