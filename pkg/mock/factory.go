package mock

import "github.com/pion/interceptor"

// Factory is a mock Factory for testing.
type Factory struct {
	NewInterceptorFn func(id string) (interceptor.Interceptor, error)
}

// NewInterceptor implements Interceptor
func (f *Factory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	return f.NewInterceptorFn(id)
}
