// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package stats

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

//nolint:cyclop
func TestInterceptor(t *testing.T) {
	t.Run("before any packets", func(t *testing.T) {
		f, err := NewInterceptor(WithLoggerFactory(logging.NewDefaultLoggerFactory()))
		assert.NoError(t, err)
		statsCh := make(chan Getter)
		f.OnNewPeerConnection(func(_ string, g Getter) {
			go func() {
				statsCh <- g
			}()
		})

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{SSRC: 0}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		var statsGetter Getter
		select {
		case statsGetter = <-statsCh:
		case <-time.After(time.Second):
			assert.FailNow(t, "expected to receive statsgetter")
		}

		assert.Equal(t, statsGetter.Get(0), &Stats{})
	})

	t.Run("records packets", func(t *testing.T) {
		mockRecorder := newMockRecorder()
		now := time.Now()
		testInterceptor, err := NewInterceptor(
			SetRecorderFactory(func(uint32, float64) Recorder {
				return mockRecorder
			}),
			SetNowFunc(func() time.Time {
				return now
			}),
		)
		assert.NoError(t, err)
		statsCh := make(chan Getter)
		testInterceptor.OnNewPeerConnection(func(_ string, g Getter) {
			go func() {
				statsCh <- g
			}()
		})

		i, err := testInterceptor.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{SSRC: 0}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		incomingRTP := &rtp.Packet{}
		incomingRTCP := []rtcp.Packet{&rtcp.RawPacket{}}
		outgoingRTP := &rtp.Packet{}
		outgoingRTCP := []rtcp.Packet{&rtcp.RawPacket{}}

		stream.ReceiveRTP(incomingRTP)
		stream.ReceiveRTCP(incomingRTCP)
		assert.NoError(t, stream.WriteRTP(outgoingRTP))
		assert.NoError(t, stream.WriteRTCP(outgoingRTCP))

		var statsGetter Getter
		select {
		case statsGetter = <-statsCh:
		case <-time.After(time.Second):
			assert.FailNow(t, "expected to receive statsgetter")
		}

		var riRTP recordedIncomingRTP
		select {
		case riRTP = <-mockRecorder.incomingRTPQueue:
		case <-time.After(time.Second):
			assert.FailNow(t, "expected to record RTP packet")
		}

		var riRTCP recordedIncomingRTCP
		select {
		case riRTCP = <-mockRecorder.incomingRTCPQueue:
		case <-time.After(time.Second):
		}

		var roRTP recordedOutgoingRTP
		select {
		case roRTP = <-mockRecorder.outgoingRTPQueue:
		case <-time.After(time.Second):
		}

		var roRTCP recordedOutgoingRTCP
		select {
		case roRTCP = <-mockRecorder.outgoingRTCPQueue:
		case <-time.After(time.Second):
		}

		assert.Equal(t, &Stats{}, statsGetter.Get(0))

		buf, err := incomingRTP.Marshal()
		assert.NoError(t, err)
		expectedIncomingRTP := recordedIncomingRTP{
			ts:   now,
			buf:  buf,
			attr: map[any]any{},
		}
		assert.Equal(t, expectedIncomingRTP, riRTP)

		buf, err = rtcp.Marshal(incomingRTCP)
		assert.NoError(t, err)
		expectedIncomingRTCP := recordedIncomingRTCP{
			ts:   now,
			buf:  buf,
			attr: map[any]any{},
		}
		assert.Equal(t, expectedIncomingRTCP, riRTCP)

		expectedOutgoingRTP := recordedOutgoingRTP{
			ts:      now,
			header:  &rtp.Header{},
			payload: outgoingRTP.Payload,
			attr:    map[any]any{},
		}
		assert.Equal(t, expectedOutgoingRTP, roRTP)

		expectedOutgoingRTCP := recordedOutgoingRTCP{
			ts:   now,
			pkts: outgoingRTCP,
			attr: map[any]any{},
		}
		assert.Equal(t, expectedOutgoingRTCP, roRTCP)
	})
}

type recordedOutgoingRTP struct {
	ts      time.Time
	header  *rtp.Header
	payload []byte
	attr    interceptor.Attributes
}

type recordedOutgoingRTCP struct {
	ts   time.Time
	pkts []rtcp.Packet
	attr interceptor.Attributes
}

type recordedIncomingRTP struct {
	ts   time.Time
	buf  []byte
	attr interceptor.Attributes
}

type recordedIncomingRTCP struct {
	ts   time.Time
	buf  []byte
	attr interceptor.Attributes
}

type mockRecorder struct {
	incomingRTPQueue  chan recordedIncomingRTP
	incomingRTCPQueue chan recordedIncomingRTCP
	outgoingRTPQueue  chan recordedOutgoingRTP
	outgoingRTCPQueue chan recordedOutgoingRTCP
}

func newMockRecorder() *mockRecorder {
	return &mockRecorder{
		incomingRTPQueue:  make(chan recordedIncomingRTP, 1),
		incomingRTCPQueue: make(chan recordedIncomingRTCP, 1),
		outgoingRTPQueue:  make(chan recordedOutgoingRTP, 1),
		outgoingRTCPQueue: make(chan recordedOutgoingRTCP, 1),
	}
}

func (r *mockRecorder) QueueIncomingRTP(ts time.Time, buf []byte, attr interceptor.Attributes) {
	r.incomingRTPQueue <- recordedIncomingRTP{
		ts:   ts,
		buf:  buf,
		attr: attr,
	}
}

func (r *mockRecorder) QueueIncomingRTCP(ts time.Time, buf []byte, attr interceptor.Attributes) {
	r.incomingRTCPQueue <- recordedIncomingRTCP{
		ts:   ts,
		buf:  buf,
		attr: attr,
	}
}

func (r *mockRecorder) QueueOutgoingRTP(ts time.Time, header *rtp.Header, payload []byte, attr interceptor.Attributes) {
	r.outgoingRTPQueue <- recordedOutgoingRTP{
		ts:      ts,
		header:  header,
		payload: payload,
		attr:    attr,
	}
}

func (r *mockRecorder) QueueOutgoingRTCP(ts time.Time, pkts []rtcp.Packet, attr interceptor.Attributes) {
	r.outgoingRTCPQueue <- recordedOutgoingRTCP{
		ts:   ts,
		pkts: pkts,
		attr: attr,
	}
}

func (r *mockRecorder) GetStats() Stats {
	return Stats{}
}

func (r *mockRecorder) Start() {}

func (r *mockRecorder) Stop() {}
