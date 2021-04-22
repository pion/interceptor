package interceptor

// Registry is a collector for interceptors.
type Registry struct {
	factories []Factory
}

// Add adds a new Interceptor to the registry. It will be wrapped by a
// SharedFactory.
func (i *Registry) Add(icpr Interceptor) {
	i.factories = append(i.factories, NewSharedFactory(icpr))
}

// AddFactory adds a Factory to the registry.
func (i *Registry) AddFactory(factory Factory) {
	i.factories = append(i.factories, factory)
}

// Build constructs a single Interceptor from a InterceptorRegistry
// TODO(jeremija) If we always expect to have a RTCPWriter bound, does it make
// sense to expect the writer to be passed through here?
func (i *Registry) Build(sessionID SessionID) (Interceptor, error) {
	if len(i.factories) == 0 {
		return &NoOp{}, nil
	}

	return NewChainFromFactories(i.factories, sessionID)
}
