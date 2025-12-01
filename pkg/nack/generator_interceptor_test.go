// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package nack

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

func TestGeneratorInterceptor(t *testing.T) {
	const interval = time.Millisecond * 10
	f, err := NewGeneratorInterceptor(
		GeneratorSize(64),
		GeneratorSkipLastN(2),
		GeneratorMaxNacksPerPacket(10),
		GeneratorInterval(interval),
		GeneratorLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	i, err := f.NewInterceptor("")
	assert.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         1,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, i)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	for _, seqNum := range []uint16{10, 11, 12, 14, 16, 18} {
		stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})

		select {
		case r := <-stream.ReadRTP():
			assert.NoError(t, r.Err)
			assert.Equal(t, seqNum, r.Packet.SequenceNumber)
		case <-time.After(50 * time.Millisecond):
			assert.FailNow(t, "receiver rtp packet not found")
		}
	}

	time.Sleep(interval * 2) // wait for at least 2 nack packets

	select {
	case <-stream.WrittenRTCP():
		// ignore the first nack, it might only contain the sequence id 13 as missing
	default:
	}

	select {
	case pkts := <-stream.WrittenRTCP():
		assert.Equal(t, 1, len(pkts), "single packet RTCP Compound Packet expected")

		p, ok := pkts[0].(*rtcp.TransportLayerNack)
		assert.True(t, ok, "TransportLayerNack rtcp packet expected, found: %T", pkts[0])

		assert.Equal(t, uint16(13), p.Nacks[0].PacketID)
		// we want packets: 13, 15 (not packet 17, because skipLastN is setReceived to 2)
		assert.Equal(t, rtcp.PacketBitmap(0b10), p.Nacks[0].LostPackets)
	case <-time.After(10 * time.Millisecond):
		assert.FailNow(t, "written rtcp packet not found")
	}
}

func TestGeneratorInterceptor_InvalidSize(t *testing.T) {
	f, _ := NewGeneratorInterceptor(GeneratorSize(5))

	_, err := f.NewInterceptor("")
	assert.Error(t, err, ErrInvalidSize)
}

func TestGeneratorInterceptor_StreamFilter(t *testing.T) {
	const interval = time.Millisecond * 10
	f, err := NewGeneratorInterceptor(
		GeneratorSize(64),
		GeneratorSkipLastN(2),
		GeneratorInterval(interval),
		GeneratorLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
		GeneratorStreamsFilter(func(info *interceptor.StreamInfo) bool {
			return info.SSRC != 1 // enable nacks only for ssrc 2
		}),
	)
	assert.NoError(t, err)

	testInterceptor, err := f.NewInterceptor("")
	assert.NoError(t, err)

	streamWithoutNacks := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         1,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, testInterceptor)
	defer func() {
		assert.NoError(t, streamWithoutNacks.Close())
	}()

	streamWithNacks := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         2,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, testInterceptor)
	defer func() {
		assert.NoError(t, streamWithNacks.Close())
	}()

	for _, seqNum := range []uint16{10, 11, 12, 14, 16, 18} {
		streamWithNacks.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})
		streamWithoutNacks.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})
	}

	time.Sleep(interval * 2) // wait for at least 2 nack packets

	// both test streams receive RTCP packets about both test streams (as they both call BindRTCPWriter), so we
	// can check only one
	rtcpStream := streamWithNacks.WrittenRTCP()

	for {
		select {
		case pkts := <-rtcpStream:
			for _, pkt := range pkts {
				if nack, isNack := pkt.(*rtcp.TransportLayerNack); isNack {
					assert.NotEqual(t, uint32(1), nack.MediaSSRC) // check there are no nacks for ssrc 1
				}
			}
		default:
			return
		}
	}
}

func TestGeneratorInterceptor_UnbindRemovesCorrespondingSSRC(t *testing.T) {
	f, err := NewGeneratorInterceptor(
		GeneratorSize(64),
	)
	assert.NoError(t, err)

	i, err := f.NewInterceptor("")
	assert.NoError(t, err)
	gen, ok := i.(*GeneratorInterceptor)
	assert.True(t, ok, "expected *GeneratorInterceptor, got %T", i)

	const ssrc = uint32(1234)

	info := &interceptor.StreamInfo{
		SSRC:         ssrc,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}

	rl, err := newReceiveLog(gen.size)
	assert.NoError(t, err)

	// make the receive log and count logs non-empty
	gen.receiveLogsMu.Lock()
	gen.receiveLogs[ssrc] = rl
	gen.nackCountLogs[ssrc] = map[uint16]uint16{
		10: 1,
		20: 2,
	}
	gen.receiveLogsMu.Unlock()

	// unbind the stream
	gen.UnbindRemoteStream(info)

	// compare them and ensure that ssrc isn't there
	gen.receiveLogsMu.Lock()
	defer gen.receiveLogsMu.Unlock()

	_, ok = gen.receiveLogs[ssrc]
	assert.False(t, ok, "ssrc should not be present in receiveLogs")

	_, ok = gen.nackCountLogs[ssrc]
	assert.False(t, ok, "ssrc should not be present in nackCountLogs")
}

// reentrantRTCPWriter tries to re-acquire GeneratorInterceptor.receiveLogsMu
// inside Write. If loop() calls Write while holding that mutex, this will
// cause a deadlock.
type reentrantRTCPWriter struct {
	n      *GeneratorInterceptor
	called chan struct{}
}

func (w *reentrantRTCPWriter) Write(pkts []rtcp.Packet, attrs interceptor.Attributes) (int, error) {
	// signal to the test that Write was entered.
	select {
	case <-w.called:
		// already closed
	default:
		close(w.called)
	}

	// re-enter the interceptor's lock
	w.n.receiveLogsMu.Lock()
	defer w.n.receiveLogsMu.Unlock()

	return len(pkts), nil
}

// this fails if loop() calls rtcpWriter.Write while holding receiveLogsMu
// but will pass if Write is called after releasing receiveLogsMu.
func TestGeneratorInterceptor_NoDeadlockWithReentrantRTCPWriter(t *testing.T) {
	const interval = time.Millisecond * 5

	f, err := NewGeneratorInterceptor(
		GeneratorSize(64),
		GeneratorInterval(interval),
	)
	assert.NoError(t, err)

	i, err := f.NewInterceptor("")
	assert.NoError(t, err)
	gen, ok := i.(*GeneratorInterceptor)
	assert.True(t, ok, "expected *GeneratorInterceptor, got %T", i)

	writer := &reentrantRTCPWriter{
		n:      gen,
		called: make(chan struct{}),
	}
	_ = gen.BindRTCPWriter(writer)

	// set receiveLog with a gap so that missingSeqNumbers()
	// returns something and causes a NACK -> Write() call.
	rl, err := newReceiveLog(gen.size)
	assert.NoError(t, err)

	gen.receiveLogsMu.Lock()
	gen.receiveLogs[1] = rl
	gen.receiveLogsMu.Unlock()

	// 100 and 102 received -> 101 is missing.
	rl.add(100)
	rl.add(102)

	// wait until the writer was actually called at least once.
	select {
	case <-writer.called:
		// good: generator loop attempted to send a NACK
	case <-time.After(time.Second):
		assert.Fail(t, "generator did not call RTCP writer")
	}

	// verify that Close() does not deadlock.
	done := make(chan struct{})
	go func() {
		_ = gen.Close()
		close(done)
	}()

	select {
	case <-done:
		// no deadlock with reentrant writer
	case <-time.After(time.Second):
		assert.Fail(t, "GeneratorInterceptor.Close deadlocked with reentrant RTCP writer")
	}
}
