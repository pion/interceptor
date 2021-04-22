package interceptor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockInterceptor struct {
	*NoOp
	sessions sessions
}

type sessions map[SessionID]struct{}

func newMockInterceptor(sess sessions) *mockInterceptor {
	return &mockInterceptor{
		NoOp:     &NoOp{},
		sessions: sess,
	}
}

func (m *mockInterceptor) BindRTCPWriter(sessionID SessionID, writer RTCPWriter) RTCPWriter {
	m.sessions[sessionID] = struct{}{}

	return writer
}

func TestRegistry_Add_Build(t *testing.T) {
	r := Registry{}
	sess := sessions{}
	i := newMockInterceptor(sess)

	r.Add(i)

	i1, err := r.Build("a")
	require.NoError(t, err)

	i2, err := r.Build("b")
	require.NoError(t, err)

	i1.BindRTCPWriter("a", nil)
	i2.BindRTCPWriter("b", nil)

	assert.Contains(t, sess, SessionID("a"), "expected session 'a'")
	assert.Contains(t, sess, SessionID("b"), "expected session 'b'")
}

func TestRegistry_AddFactory_Build(t *testing.T) {
	r := Registry{}

	interceptorsBySession := map[SessionID]*mockInterceptor{}

	factory := FactoryFunc(func(sessionID SessionID) (Interceptor, error) {
		sess := sessions{}
		i := newMockInterceptor(sess)
		interceptorsBySession[sessionID] = i
		return i, nil
	})

	r.AddFactory(factory)

	i1, err := r.Build("a")
	require.NoError(t, err)

	i2, err := r.Build("b")
	require.NoError(t, err)

	i1.BindRTCPWriter("a", nil)
	i2.BindRTCPWriter("b", nil)

	assert.Contains(t, interceptorsBySession, SessionID("a"), "expected session 'a'")
	assert.Contains(t, interceptorsBySession, SessionID("b"), "expected session 'b'")
	assert.NotEqual(t, interceptorsBySession["a"], interceptorsBySession["b"],
		"expected two separate interceptor instances")
}
