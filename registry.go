package interceptor

// InterceptorRegistry is a collector for interceptors.
type Registry struct {
	interceptors []Interceptor
}

// Add adds a new Interceptor to the registry.
func (i *Registry) Add(icpr Interceptor) {
	i.interceptors = append(i.interceptors, icpr)
}

func (i *Registry) build() Interceptor {
	if len(i.interceptors) == 0 {
		return &NoOp{}
	}

	return NewChain(i.interceptors)
}
