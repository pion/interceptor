package rfc8888

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestInterceptor(t *testing.T) {
	t.Run("before any packet", func(t *testing.T) {
		f, err := NewSenderInterceptor()
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC: 123456,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		var pkts []rtcp.Packet
		select {
		case pkts = <-stream.WrittenRTCP():
		case <-time.After(300 * time.Millisecond):
		}
		assert.Equal(t, len(pkts), 0)
	})

	t.Run("after RTP packets", func(t *testing.T) {
		f, err := NewSenderInterceptor()
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC: 123456,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		for i := 0; i < 10; i++ {
			stream.ReceiveRTP(&rtp.Packet{
				Header: rtp.Header{
					Version:          0,
					Padding:          false,
					Extension:        false,
					Marker:           false,
					PayloadType:      0,
					SequenceNumber:   uint16(i),
					Timestamp:        0,
					SSRC:             123456,
					CSRC:             []uint32{},
					ExtensionProfile: 0,
					Extensions:       []rtp.Extension{},
				},
				Payload:     []byte{},
				PaddingSize: 0,
			})
		}

		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, len(pkts), 1)
		fb, ok := pkts[0].(*rtcp.CCFeedbackReport)
		assert.True(t, ok)
		assert.Equal(t, 1, len(fb.ReportBlocks))
		assert.Equal(t, uint32(123456), fb.ReportBlocks[0].MediaSSRC)
		assert.Equal(t, 10, len(fb.ReportBlocks[0].MetricBlocks))
	})

	t.Run("different delays between RTP packets", func(t *testing.T) {
		mNow := &test.MockTime{}
		mTick := &test.MockTicker{
			C: make(chan time.Time),
		}
		f, err := NewSenderInterceptor(
			SenderTicker(func(d time.Duration) ticker {
				return mTick
			}),
			SenderNow(mNow.Now),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC: 123456,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		zero := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

		delays := []time.Duration{
			0,
			250 * time.Millisecond,
			500 * time.Millisecond,
			time.Second,
		}
		for i, d := range delays {
			mNow.SetNow(zero.Add(d))
			stream.ReceiveRTP(&rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(i),
					SSRC:           123456,
				},
			})
			select {
			case r := <-stream.ReadRTP():
				assert.NoError(t, r.Err)
			case <-time.After(10 * time.Millisecond):
				t.Fatal("receiver rtp packet not found")
			}
		}
		mTick.Tick(zero.Add(time.Second))
		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(pkts))
		ccfb, ok := pkts[0].(*rtcp.CCFeedbackReport)
		assert.True(t, ok)
		assert.Equal(t, uint32(1<<16), ccfb.ReportTimestamp)
		assert.Equal(t, 1, len(ccfb.ReportBlocks))
		assert.Equal(t, uint32(123456), ccfb.ReportBlocks[0].MediaSSRC)
		assert.Equal(t, 4, len(ccfb.ReportBlocks[0].MetricBlocks))
		assert.Equal(t, uint16(0), ccfb.ReportBlocks[0].BeginSequence)
		assert.Equal(t, []rtcp.CCFeedbackMetricBlock{
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 1024,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 512 + 256,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 512,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 0,
			},
		}, ccfb.ReportBlocks[0].MetricBlocks)
	})

	t.Run("packet loss", func(t *testing.T) {
		mNow := &test.MockTime{}
		mTick := &test.MockTicker{
			C: make(chan time.Time),
		}
		f, err := NewSenderInterceptor(
			SenderTicker(func(d time.Duration) ticker {
				return mTick
			}),
			SenderNow(mNow.Now),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC: 123456,
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		zero := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

		sequenceNumberToDelay := map[int]int{
			0:  0,
			1:  125,
			4:  250,
			8:  500,
			9:  750,
			10: 1000,
		}
		for i := 0; i <= 10; i++ {
			if _, ok := sequenceNumberToDelay[i]; !ok {
				continue
			}
			mNow.SetNow(zero.Add(time.Duration(sequenceNumberToDelay[i]) * time.Millisecond))
			stream.ReceiveRTP(&rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(i),
					SSRC:           123456,
				},
			})
			select {
			case r := <-stream.ReadRTP():
				assert.NoError(t, r.Err)
			case <-time.After(10 * time.Millisecond):
				t.Fatal("receiver rtp packet not found")
			}
		}
		mTick.Tick(zero.Add(time.Second))
		pkts := <-stream.WrittenRTCP()
		assert.Equal(t, 1, len(pkts))
		ccfb, ok := pkts[0].(*rtcp.CCFeedbackReport)
		assert.True(t, ok)
		assert.Equal(t, uint32(1<<16), ccfb.ReportTimestamp)
		assert.Equal(t, 1, len(ccfb.ReportBlocks))
		assert.Equal(t, uint32(123456), ccfb.ReportBlocks[0].MediaSSRC)
		assert.Equal(t, 11, len(ccfb.ReportBlocks[0].MetricBlocks))
		assert.Equal(t, uint16(0), ccfb.ReportBlocks[0].BeginSequence)
		assert.Equal(t, []rtcp.CCFeedbackMetricBlock{
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 1024,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 1024 - 128,
			},
			{
				Received:          false,
				ECN:               0,
				ArrivalTimeOffset: 0,
			},
			{
				Received:          false,
				ECN:               0,
				ArrivalTimeOffset: 0,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 1024 - 256,
			},
			{
				Received:          false,
				ECN:               0,
				ArrivalTimeOffset: 0,
			},
			{
				Received:          false,
				ECN:               0,
				ArrivalTimeOffset: 0,
			},
			{
				Received:          false,
				ECN:               0,
				ArrivalTimeOffset: 0,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 512,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 256,
			},
			{
				Received:          true,
				ECN:               0,
				ArrivalTimeOffset: 0,
			},
		}, ccfb.ReportBlocks[0].MetricBlocks)
	})
}
