package interceptor

// Factory defines a way to create interface
type Factory interface {
	NewInterceptor(SessionID) (Interceptor, error)
}

// FactoryFunc defines a simpler interface to use when creating a Factory,
// similar to http.HandlerFunc.
type FactoryFunc func(SessionID) (Interceptor, error)

// NewInterceptor implements Factory.
func (f FactoryFunc) NewInterceptor(sessionID SessionID) (Interceptor, error) {
	return f(sessionID)
}

// SharedFactory is the simplest implementation of Factory, it always returns
// the same, already initialized Interceptor and expects it to be able to
// handle streams from different contexts.
type SharedFactory struct {
	interceptor Interceptor
}

// NewSharedFactory creates a new instance of SharedFactory.
func NewSharedFactory(interceptor Interceptor) *SharedFactory {
	return &SharedFactory{
		interceptor: interceptor,
	}
}

// NewInterceptor always returns the same interceptor and ignores the SessionID
// argument.
func (s *SharedFactory) NewInterceptor(_ SessionID) (Interceptor, error) {
	return s.interceptor, nil
}
