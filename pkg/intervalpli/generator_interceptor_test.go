// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package intervalpli

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
)

func TestPLIGeneratorInterceptor_Unsupported(t *testing.T) {
	i, err := NewGeneratorInterceptor(
		GeneratorInterval(time.Millisecond*10),
		GeneratorLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.Nil(t, err)

	streamSSRC := uint32(123456)
	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:     streamSSRC,
		MimeType: "video/h264",
	}, i)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	timeout := time.NewTimer(100 * time.Millisecond)
	defer timeout.Stop()
	select {
	case <-timeout.C:
		return
	case <-stream.WrittenRTCP():
		assert.FailNow(t, "should not receive any PIL")
	}
}

func TestPLIGeneratorInterceptor(t *testing.T) {
	generatorInterceptor, err := NewGeneratorInterceptor(
		GeneratorInterval(time.Second*1),
		GeneratorLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.Nil(t, err)

	streamSSRC := uint32(123456)
	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      streamSSRC,
		ClockRate: 90000,
		MimeType:  "video/h264",
		RTCPFeedback: []interceptor.RTCPFeedback{
			{Type: "nack", Parameter: "pli"},
		},
	}, generatorInterceptor)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	pkts := <-stream.WrittenRTCP()
	assert.Equal(t, len(pkts), 1)
	sr, ok := pkts[0].(*rtcp.PictureLossIndication)
	assert.True(t, ok)
	assert.Equal(t, &rtcp.PictureLossIndication{MediaSSRC: streamSSRC}, sr)

	// Should not have another packet immediately...
	func() {
		timeout := time.NewTimer(100 * time.Millisecond)
		defer timeout.Stop()
		select {
		case <-timeout.C:
			return
		case <-stream.WrittenRTCP():
			assert.FailNow(t, "should not receive any PIL")
		}
	}()

	// ... but should receive one 1sec later.
	pkts = <-stream.WrittenRTCP()
	assert.Equal(t, len(pkts), 1)
	sr, ok = pkts[0].(*rtcp.PictureLossIndication)
	assert.True(t, ok)
	assert.Equal(t, &rtcp.PictureLossIndication{MediaSSRC: streamSSRC}, sr)
}
