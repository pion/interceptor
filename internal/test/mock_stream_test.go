// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package test

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

//nolint:cyclop
func TestMockStream(t *testing.T) {
	mockStream := NewMockStream(&interceptor.StreamInfo{}, &interceptor.NoOp{})

	assert.NoError(t, mockStream.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{}}))

	select {
	case <-mockStream.WrittenRTCP():
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "rtcp packet written but not found")
	}
	select {
	case <-mockStream.WrittenRTCP():
		assert.Fail(t, "single rtcp packet written, but multiple found")
	case <-time.After(10 * time.Millisecond):
	}

	assert.NoError(t, mockStream.WriteRTP(&rtp.Packet{}))

	select {
	case <-mockStream.WrittenRTP():
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "rtp packet written but not found")
	}
	select {
	case <-mockStream.WrittenRTP():
		assert.Fail(t, "single rtp packet written, but multiple found")
	case <-time.After(10 * time.Millisecond):
	}

	mockStream.ReceiveRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{}})
	select {
	case r := <-mockStream.ReadRTCP():
		assert.NoError(t, r.Err, "read rtcp returned error")
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "rtcp packet received but not read")
	}
	select {
	case r := <-mockStream.ReadRTCP():
		assert.Fail(t, "single rtcp packet received, but multiple read: %v", r)
	case <-time.After(10 * time.Millisecond):
	}

	mockStream.ReceiveRTP(&rtp.Packet{})
	select {
	case r := <-mockStream.ReadRTP():
		assert.NoError(t, r.Err, "read rtp returned error")
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "rtp packet received but not read")
	}
	select {
	case r := <-mockStream.ReadRTP():
		assert.Fail(t, "single rtp packet received, but multiple read: %v", r)
	case <-time.After(10 * time.Millisecond):
	}

	assert.NoError(t, mockStream.Close())
}
