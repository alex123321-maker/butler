package transport

import (
	"fmt"
	"sync"
)

// ProviderFactory constructs a ModelProvider from an opaque config value.
// Each concrete provider package registers its own factory via RegisterProvider.
type ProviderFactory func(config any) (ModelProvider, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]ProviderFactory)
)

// RegisterProvider registers a named provider factory.
// It is intended to be called from init() in each concrete provider package.
// Panics if name is empty or already registered.
func RegisterProvider(name string, factory ProviderFactory) {
	if name == "" {
		panic("transport: RegisterProvider called with empty name")
	}
	if factory == nil {
		panic("transport: RegisterProvider called with nil factory")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("transport: provider %q already registered", name))
	}
	registry[name] = factory
}

// NewProvider creates a ModelProvider by looking up the named factory.
// The config value is passed through to the factory; the concrete provider
// package defines the expected config type.
func NewProvider(name string, config any) (ModelProvider, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("transport: unknown provider %q", name)
	}
	return factory(config)
}

// RegisteredProviders returns the names of all registered providers.
func RegisteredProviders() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
