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
	"github.com/stretchr/testify/require"
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
		incomingRTCP := []rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: 0}}
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
		rtcpAttributes := interceptor.Attributes{}
		_, err = rtcpAttributes.GetRTCPPackets(buf)
		assert.NoError(t, err)
		expectedIncomingRTCP := recordedIncomingRTCP{
			ts:   now,
			buf:  buf,
			attr: rtcpAttributes,
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
	stops             int
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

func (r *mockRecorder) Stop() { r.stops++ }

func newTestInterceptor(t *testing.T, recorderFactory RecorderFactory) *Interceptor {
	t.Helper()
	factory, err := NewInterceptor(SetRecorderFactory(recorderFactory))
	require.NoError(t, err)
	instance, err := factory.NewInterceptor("")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, instance.Close()) })

	return instance.(*Interceptor) //nolint:forcetypeassert
}

func TestInterceptor_UnbindStreams(t *testing.T) {
	var recorders []*mockRecorder
	stats := newTestInterceptor(t, func(uint32, float64) Recorder {
		recorder := newMockRecorder()
		recorders = append(recorders, recorder)

		return recorder
	})
	for ssrc := uint32(1); ssrc <= 1000; ssrc++ {
		info := &interceptor.StreamInfo{SSRC: ssrc}
		stats.BindRemoteStream(info, nil)
		stats.BindLocalStream(info, nil)
		stats.UnbindRemoteStream(info)
		require.NotNil(t, stats.Get(ssrc))
		require.Zero(t, recorders[len(recorders)-1].stops)
		stats.UnbindLocalStream(info)
		stats.UnbindLocalStream(info)
	}
	require.Empty(t, stats.recorders)
	info := &interceptor.StreamInfo{SSRC: 1}
	stats.BindRemoteStream(info, nil)
	require.Len(t, recorders, 1001)
	require.NoError(t, stats.Close())
	for _, recorder := range recorders {
		require.Equal(t, 1, recorder.stops)
	}
}

func TestInterceptor_RouteIncomingRTCP(t *testing.T) {
	for _, returned := range []interceptor.Attributes{nil, {"reader": true}} {
		recorders := map[uint32]*mockRecorder{}
		stats := newTestInterceptor(t, func(ssrc uint32, _ float64) Recorder {
			recorder := newMockRecorder()
			recorder.incomingRTCPQueue = make(chan recordedIncomingRTCP, 10)
			recorders[ssrc] = recorder

			return recorder
		})
		for ssrc := uint32(1); ssrc <= 100; ssrc++ {
			stats.BindLocalStream(&interceptor.StreamInfo{SSRC: ssrc}, nil)
		}
		packets := []rtcp.Packet{
			&rtcp.ReceiverReport{Reports: []rtcp.ReceptionReport{{SSRC: 1}}},
			&rtcp.SenderReport{SSRC: 2, Reports: []rtcp.ReceptionReport{{SSRC: 1}}},
			&rtcp.ExtendedReport{SenderSSRC: 3, Reports: []rtcp.ReportBlock{
				&rtcp.DLRRReportBlock{Reports: []rtcp.DLRRReport{{SSRC: 1}}},
			}},
			&rtcp.PictureLossIndication{MediaSSRC: 1},
			&rtcp.PictureLossIndication{MediaSSRC: 999},
		}
		wire, err := rtcp.Marshal(packets)
		require.NoError(t, err)
		parsed, err := returned.GetRTCPPackets(wire)
		require.NoError(t, err)
		reader := stats.BindRTCPReader(interceptor.RTCPReaderFunc(
			func(buf []byte, _ interceptor.Attributes) (int, interceptor.Attributes, error) {
				return copy(buf, wire), returned, nil
			},
		))
		n, attributes, err := reader.Read(make([]byte, len(wire)+100), nil)
		require.NoError(t, err)
		require.Equal(t, len(wire), n)
		shared, err := attributes.GetRTCPPackets(nil)
		require.NoError(t, err)
		require.Len(t, shared, len(packets))
		if returned != nil {
			require.Equal(t, true, attributes["reader"])
			require.Same(t, parsed[0], shared[0])
		}
		for ssrc, recorder := range recorders {
			if ssrc > 3 {
				require.Empty(t, recorder.incomingRTCPQueue)

				continue
			}
			require.Len(t, recorder.incomingRTCPQueue, 1)
			recorded := <-recorder.incomingRTCPQueue
			require.Equal(t, wire, recorded.buf)
			decoded, err := recorded.attr.GetRTCPPackets(nil)
			require.NoError(t, err)
			require.Len(t, decoded, len(packets))
			require.Same(t, shared[0], decoded[0])
		}
	}
}
