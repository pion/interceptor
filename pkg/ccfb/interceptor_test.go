// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package ccfb

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

//nolint:maintidx,cyclop
func TestInterceptor(t *testing.T) {
	t.Run("before any packet", func(t *testing.T) {
		f, err := NewSenderInterceptor(WithLoggerFactory(logging.NewDefaultLoggerFactory()))
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

		for i := range 10 {
			stream.ReceiveRTP(&rtp.Packet{
				Header: rtp.Header{
					Version:          0,
					Padding:          false,
					Extension:        false,
					Marker:           false,
					PayloadType:      0,
					SequenceNumber:   uint16(i), //nolint:gosec // G115
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
			SenderTicker(func(time.Duration) ticker {
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
					SequenceNumber: uint16(i), //nolint:gosec // G115
					SSRC:           123456,
				},
			})
			select {
			case r := <-stream.ReadRTP():
				assert.NoError(t, r.Err)
			case <-time.After(10 * time.Millisecond):
				assert.Fail(t, "receiver rtp packet not found")
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
			SenderTicker(func(time.Duration) ticker {
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
					SequenceNumber: uint16(i), //nolint:gosec // G115
					SSRC:           123456,
				},
			})
			select {
			case r := <-stream.ReadRTP():
				assert.NoError(t, r.Err)
			case <-time.After(10 * time.Millisecond):
				assert.Fail(t, "receiver rtp packet not found")
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

func TestReadAfterClose(t *testing.T) {
	f, err := NewSenderInterceptor()
	assert.NoError(t, err)

	intcp, err := f.NewInterceptor("")
	assert.NoError(t, err)

	intcp.BindRTCPWriter(interceptor.RTCPWriterFunc(
		func(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
			return 0, nil
		},
	))

	raw, err := (&rtp.Packet{
		Header:  rtp.Header{Version: 2, SequenceNumber: 1, SSRC: 123456},
		Payload: []byte{},
	}).Marshal()
	assert.NoError(t, err)

	reader := intcp.BindRemoteStream(&interceptor.StreamInfo{SSRC: 123456}, interceptor.RTPReaderFunc(
		func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
			return copy(b, raw), a, nil
		},
	))

	assert.NoError(t, intcp.Close())

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, err := reader.Read(make([]byte, 1500), nil)
		assert.ErrorIs(t, err, errClosed)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		assert.Fail(t, "read after close blocked")
	}
}

func TestConcurrentClose(t *testing.T) {
	f, err := NewSenderInterceptor()
	assert.NoError(t, err)

	intcp, err := f.NewInterceptor("")
	assert.NoError(t, err)

	intcp.BindRTCPWriter(interceptor.RTCPWriterFunc(
		func(_ []rtcp.Packet, _ interceptor.Attributes) (int, error) {
			return 0, nil
		},
	))

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, intcp.Close())
		}()
	}
	wg.Wait()
}

func TestInterceptorECN(t *testing.T) {
	mTick := &test.MockTicker{C: make(chan time.Time)}
	factory, err := NewSenderInterceptor(SenderTicker(func(time.Duration) ticker { return mTick }))
	assert.NoError(t, err)
	intcp, err := factory.NewInterceptor("")
	assert.NoError(t, err)
	defer func() { assert.NoError(t, intcp.Close()) }()

	reports := make(chan []rtcp.Packet, 1)
	intcp.BindRTCPWriter(interceptor.RTCPWriterFunc(
		func(packets []rtcp.Packet, _ interceptor.Attributes) (int, error) {
			reports <- packets

			return 0, nil
		},
	))
	header := rtp.Header{Version: 2, SSRC: 123456}
	reader := intcp.BindRemoteStream(&interceptor.StreamInfo{SSRC: header.SSRC}, interceptor.RTPReaderFunc(
		func(buf []byte, attrs interceptor.Attributes) (int, interceptor.Attributes, error) {
			n, marshalErr := header.MarshalTo(buf)
			header.SequenceNumber++

			return n, attrs, marshalErr
		},
	))
	cases := []struct {
		attrs interceptor.Attributes
		ecn   rtcp.ECN
	}{
		{interceptor.Attributes{interceptor.ECNKey: rtcp.ECNNonECT}, rtcp.ECNNonECT},
		{interceptor.Attributes{interceptor.ECNKey: rtcp.ECNECT1}, rtcp.ECNECT1},
		{interceptor.Attributes{interceptor.ECNKey: rtcp.ECNECT0}, rtcp.ECNECT0},
		{interceptor.Attributes{interceptor.ECNKey: rtcp.ECNCE}, rtcp.ECNCE},
		{nil, rtcp.ECNNonECT},
		{interceptor.Attributes{interceptor.ECNKey: byte(3)}, rtcp.ECNNonECT},
		{interceptor.Attributes{"ECN": rtcp.ECNCE, int(interceptor.ECNKey): rtcp.ECNCE}, rtcp.ECNNonECT},
	}
	for _, testCase := range cases {
		_, _, err = reader.Read(make([]byte, 1500), testCase.attrs)
		assert.NoError(t, err)
	}
	mTick.Tick(time.Now())

	select {
	case packets := <-reports:
		if !assert.Len(t, packets, 1) {
			return
		}
		report, ok := packets[0].(*rtcp.CCFeedbackReport)
		if !assert.True(t, ok) || !assert.Len(t, report.ReportBlocks, 1) {
			return
		}
		metrics := report.ReportBlocks[0].MetricBlocks
		if !assert.Len(t, metrics, len(cases)) {
			return
		}
		for i, testCase := range cases {
			assert.Equal(t, testCase.ecn, metrics[i].ECN)
		}
	case <-time.After(time.Second):
		assert.Fail(t, "ECN feedback report not received")
	}
}
