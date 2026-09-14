// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package report

import (
	"errors"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

var errWriteFailed = errors.New("write failed")

//nolint:maintidx
func TestSenderInterceptor(t *testing.T) {
	t.Run("before any packet", func(t *testing.T) {
		mt := &test.MockTime{}
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			WithSenderLoggerFactory(logging.NewDefaultLoggerFactory()),
			SenderNow(mt.Now),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		mt.SetNow(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
		select {
		case pkts := <-stream.WrittenRTCP():
			assert.Fail(t, "no sender report expected before any RTP packet", "%+v", pkts)
		case <-time.After(time.Millisecond * 200):
		}
	})

	t.Run("after RTP packets", func(t *testing.T) {
		mt := &test.MockTime{}
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mt.Now),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		for i := range 10 {
			assert.NoError(t, stream.WriteRTP(&rtp.Packet{
				Header:  rtp.Header{SequenceNumber: uint16(i)}, //nolint:gosec // G115
				Payload: []byte("\x00\x00"),
			}))
		}

		mt.SetNow(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, len(pkts), 1)
		sr, ok := pkts[0].(*rtcp.SenderReport)
		assert.True(t, ok)
		assert.Equal(t, &rtcp.SenderReport{
			SSRC:        123456,
			NTPTime:     ntp.ToNTP(mt.Now()),
			RTPTime:     2269117121,
			PacketCount: 10,
			OctetCount:  20,
		}, sr)
	})

	t.Run("out of order RTP packets", func(t *testing.T) {
		mt := &test.MockTime{}
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mt.Now),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		// Write several packets
		for i := range 10 {
			assert.NoError(t, stream.WriteRTP(&rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(i), //nolint:gosec // G115
					Timestamp:      uint32(i), //nolint:gosec // G115
				},
				Payload: []byte("\x00\x00"),
			}))
		}

		// Skip a packet, then redeliver it out-of-order
		assert.NoError(t, stream.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: 12,
				Timestamp:      12,
			},
			Payload: []byte("\x00\x00"),
		}))
		assert.NoError(t, stream.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: 11,
				Timestamp:      11,
			},
			Payload: []byte("\x00\x00"),
		}))

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, len(pkts), 1)
		sr, ok := pkts[0].(*rtcp.SenderReport)
		assert.True(t, ok)
		// The out-of-order packet is included in PacketCount and OctetCount, but the RTP
		// timestamp of the last in-order packet is used for RTPTime
		assert.Equal(t, &rtcp.SenderReport{
			SSRC:        123456,
			NTPTime:     ntp.ToNTP(mt.Now()),
			RTPTime:     12,
			PacketCount: 12,
			OctetCount:  24,
		}, sr)
	})

	t.Run("out of order RTP packets with SenderUseLatestPacket", func(t *testing.T) {
		mt := &test.MockTime{}
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mt.Now),
			SenderUseLatestPacket(),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		// Write several packets
		for i := range 10 {
			assert.NoError(t, stream.WriteRTP(&rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(i), //nolint:gosec // G115
					Timestamp:      uint32(i), //nolint:gosec // G115
				},
				Payload: []byte("\x00\x00"),
			}))
		}

		// Skip a packet, then redeliver it out-of-order
		assert.NoError(t, stream.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: 12,
				Timestamp:      12,
			},
			Payload: []byte("\x00\x00"),
		}))
		assert.NoError(t, stream.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: 11,
				Timestamp:      11,
			},
			Payload: []byte("\x00\x00"),
		}))

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, len(pkts), 1)
		sr, ok := pkts[0].(*rtcp.SenderReport)
		assert.True(t, ok)
		// The out-of-order packet *is*  used for RTPTime
		assert.Equal(t, &rtcp.SenderReport{
			SSRC:        123456,
			NTPTime:     ntp.ToNTP(mt.Now()),
			RTPTime:     11,
			PacketCount: 12,
			OctetCount:  24,
		}, sr)
	})

	t.Run("inject ticker", func(t *testing.T) {
		mNow := &test.MockTime{}
		mTick := &test.MockTicker{
			C: make(chan time.Time),
		}
		advanceTicker := func() {
			mNow.SetNow(mNow.Now().Add(50 * time.Millisecond))
			mTick.Tick(mNow.Now())
		}
		loopStarted := make(chan struct{})
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mNow.Now),
			SenderTicker(func(time.Duration) Ticker { return mTick }),
			enableStartTracking(loopStarted),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		<-loopStarted
		assert.NoError(t, stream.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: 0}, Payload: []byte("\x00\x00")}))
		for range 5 {
			advanceTicker()
			pkts := <-stream.WrittenRTCP()
			assert.Equal(t, len(pkts), 1)
		}
	})

	t.Run("first report only after first RTP packet", func(t *testing.T) {
		mNow := &test.MockTime{}
		mNow.SetNow(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
		mTick := &test.MockTicker{
			C: make(chan time.Time),
		}
		loopStarted := make(chan struct{})
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mNow.Now),
			SenderTicker(func(time.Duration) Ticker { return mTick }),
			enableStartTracking(loopStarted),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		<-loopStarted
		mTick.Tick(mNow.Now())
		select {
		case pkts := <-stream.WrittenRTCP():
			assert.Fail(t, "no sender report expected before any RTP packet", "%+v", pkts)
		case <-time.After(time.Millisecond * 200):
		}

		assert.NoError(t, stream.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{SequenceNumber: 7, Timestamp: 90000},
			Payload: []byte("\x00\x00\x00"),
		}))
		mTick.Tick(mNow.Now())
		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, len(pkts), 1)
		sr, ok := pkts[0].(*rtcp.SenderReport)
		assert.True(t, ok)
		assert.Equal(t, &rtcp.SenderReport{
			SSRC:        123456,
			NTPTime:     ntp.ToNTP(mNow.Now()),
			RTPTime:     90000,
			PacketCount: 1,
			OctetCount:  3,
		}, sr)
	})

	t.Run("loop keeps running after write error", func(t *testing.T) {
		mNow := &test.MockTime{}
		mTick := &test.MockTicker{
			C: make(chan time.Time),
		}
		loopStarted := make(chan struct{})
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mNow.Now),
			SenderTicker(func(time.Duration) Ticker { return mTick }),
			enableStartTracking(loopStarted),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, i.Close())
		}()

		writes := make(chan struct{}, 10)
		i.BindRTCPWriter(interceptor.RTCPWriterFunc(func([]rtcp.Packet, interceptor.Attributes) (int, error) {
			writes <- struct{}{}

			return 0, errWriteFailed
		}))
		rtpWriter := i.BindLocalStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, interceptor.RTPWriterFunc(func(*rtp.Header, []byte, interceptor.Attributes) (int, error) {
			return 0, nil
		}))
		_, err = rtpWriter.Write(&rtp.Header{SequenceNumber: 0}, []byte("\x00\x00"), nil)
		assert.NoError(t, err)

		<-loopStarted
		for range 2 {
			mTick.Tick(mNow.Now())
			<-writes
		}
	})
	t.Run("unexpected stream type is skipped", func(t *testing.T) {
		mNow := &test.MockTime{}
		mTick := &test.MockTicker{
			C: make(chan time.Time),
		}
		loopStarted := make(chan struct{})
		f, err := NewSenderInterceptor(
			SenderInterval(time.Millisecond*50),
			SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			SenderNow(mNow.Now),
			SenderTicker(func(time.Duration) Ticker { return mTick }),
			enableStartTracking(loopStarted),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		si, ok := i.(*SenderInterceptor)
		assert.True(t, ok)
		si.streams.Store(uint32(999), "not a senderStream")

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC:      123456,
			ClockRate: 90000,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		<-loopStarted
		assert.NoError(t, stream.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: 0}, Payload: []byte("\x00\x00")}))
		mTick.Tick(mNow.Now())

		// The bogus entry is skipped and the real stream still reports.
		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, len(pkts), 1)
		_, ok = pkts[0].(*rtcp.SenderReport)
		assert.True(t, ok)
	})
}
