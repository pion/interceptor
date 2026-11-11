// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package jitterbuffer implements a buffer for RTP packets designed to help
// counteract non-deterministic sources of latency
package jitterbuffer

import (
	"errors"

	"github.com/pion/interceptor/internal/rtpbuffer"
	"github.com/pion/rtp"
)

// State tracks a JitterBuffer as either Buffering or Emitting.
type State uint16

// Event represents all events a JitterBuffer can emit.
type Event string

var (
	// ErrBufferUnderrun is returned when the buffer has no items.
	ErrBufferUnderrun = errors.New("invalid Peek: Empty jitter buffer")
	// ErrPopWhileBuffering is returned if a jitter buffer is not in a playback state.
	ErrPopWhileBuffering = errors.New("attempt to pop while buffering")
)

const (
	// Buffering is the state when the jitter buffer has not started emitting yet,
	// or has hit an underflow and needs to re-buffer packets.
	Buffering State = iota
	//  Emitting is the state when the jitter buffer is operating nominally
	Emitting
)

const (
	// StartBuffering is emitted when the buffer receives its first packet.
	StartBuffering Event = "startBuffering"
	// BeginPlayback is emitted when the buffer has satisfied its buffer length.
	BeginPlayback = "playing"
	// BufferUnderflow is emitted when the buffer does not have enough packets to Pop.
	BufferUnderflow = "underflow"
	// BufferOverflow is emitted when the buffer has exceeded its limit.
	BufferOverflow = "overflow"
)

func (jbs State) String() string {
	switch jbs {
	case Buffering:
		return "Buffering"
	case Emitting:
		return "Emitting"
	}

	return "unknown"
}

type (
	// Option will Override JitterBuffer's defaults.
	Option func(jb *JitterBuffer)
	// EventListener will be called when the corresponding Event occurs.
	EventListener func(event Event, jb *JitterBuffer)
)

// A JitterBuffer will accept Pushed packets, put them in sequence number
// order, and allows removing in either sequence number order or via a
// provided timestamp.
//
// A JitterBuffer is not safe for concurrent use; callers sharing one across
// goroutines must synchronize access themselves.
type JitterBuffer struct {
	packetFactory    rtpbuffer.PacketFactory
	reorderBuffer    *rtpbuffer.RTPBuffer
	playbackBuffer   *RingBuffer
	minStartCount    uint16
	overflowLen      uint16
	copyPackets      bool
	lastSequence     uint16
	expectedSequence uint16
	playoutHead      uint16
	playoutReady     bool
	state            State
	stats            Stats
	listeners        map[Event][]EventListener
}

// Stats Track interesting statistics for the life of this JitterBuffer
// outOfOrderCount will provide the number of times a packet was Pushed
//
//	without its predecessor being present
//
// underflowCount will provide the count of attempts to Pop an empty buffer
// overflowCount will track the number of times the jitter buffer exceeds its limit.
type Stats struct {
	outOfOrderCount uint32
	underflowCount  uint32
	overflowCount   uint32
}

var (
	// ErrInvalidOperation may be returned if a Pop or Find operation is performed on an playback buffer.
	ErrInvalidOperation = errors.New("attempt to find or pop on an empty list")
	// ErrNotFound will be returned if the packet cannot be found in the playblack buffer.
	ErrNotFound = errors.New("packet not found")
)

// New will initialize a jitter buffer and its associated statistics.
func New(opts ...Option) *JitterBuffer {
	jb := &JitterBuffer{
		state:         Buffering,
		stats:         Stats{0, 0, 0},
		minStartCount: 50,
		overflowLen:   1024,
		listeners:     make(map[Event][]EventListener),
	}

	for _, o := range opts {
		o(jb)
	}

	jb.minStartCount = min(jb.minStartCount, jb.overflowLen)

	if jb.copyPackets {
		jb.packetFactory = rtpbuffer.NewPacketFactoryCopy()
	} else {
		jb.packetFactory = &rtpbuffer.PacketFactoryNoOp{}
	}
	jb.reorderBuffer, _ = rtpbuffer.NewRTPBuffer(jb.overflowLen)
	jb.playbackBuffer = NewRingBuffer(jb.overflowLen)

	return jb
}

// WithMinimumPacketCount will set the required number of packets to be received before
// any attempt to pop a packet can succeed. The default is 50. A count larger than
// the buffer size (see WithSize) is lowered to the buffer size.
func WithMinimumPacketCount(count uint16) Option {
	return func(jb *JitterBuffer) {
		jb.minStartCount = count
	}
}

// WithSize sets how many sequence numbers the buffer can hold. It is rounded
// up to a power of two and capped at 32768 (half the sequence number space).
// Zero keeps the default of 1024. The minimum packet count is lowered to the
// resulting size if it is larger.
func WithSize(size uint16) Option {
	return func(jb *JitterBuffer) {
		if size != 0 {
			jb.overflowLen = nextPowerOfTwo(size)
		}
	}
}

const maxSize = 1 << 15

func nextPowerOfTwo(v uint16) uint16 {
	if v > maxSize {
		return maxSize
	}
	n := uint16(1)
	for n < v {
		n <<= 1
	}

	return n
}

// WithCopy makes Push copy each packet's header and payload into pooled
// buffers, so callers may reuse their packet memory once Push returns.
// Packets returned by the buffer are independent copies. Packets with
// payloads larger than 1460 bytes are dropped in this mode.
func WithCopy() Option {
	return func(jb *JitterBuffer) {
		jb.copyPackets = true
	}
}

// DisableCopy restores the default of buffering the caller's packets without
// copying, undoing an earlier WithCopy. Callers must not reuse a pushed
// packet's memory while the buffer may still return it.
func DisableCopy() Option {
	return func(jb *JitterBuffer) {
		jb.copyPackets = false
	}
}

// Listen will register an event listener
// The jitter buffer may emit events correspnding, interested listerns should
// look at Event for available events.
func (jb *JitterBuffer) Listen(event Event, cb EventListener) {
	jb.listeners[event] = append(jb.listeners[event], cb)
}

// PlayoutHead returns the SequenceNumber that will be attempted to Pop next.
func (jb *JitterBuffer) PlayoutHead() uint16 {
	return jb.playoutHead
}

// SetPlayoutHead allows you to manually specify the packet you wish to pop next
// If you have encountered a packet that hasn't resolved you can skip it.
func (jb *JitterBuffer) SetPlayoutHead(playoutHead uint16) {
	jb.playoutHead = playoutHead
	jb.expectedSequence = playoutHead
	jb.drain()
}

func (jb *JitterBuffer) updateStats(lastPktSeqNo uint16) {
	// If we have at least one packet, and the next packet being pushed in is not
	// at the expected sequence number increment the out of order count
	if jb.reorderBuffer.Started() && lastPktSeqNo != (jb.lastSequence+1) {
		jb.stats.outOfOrderCount++
	}
	jb.lastSequence = lastPktSeqNo
}

// Push an RTP packet into the jitter buffer. Unless WithCopy is set this does
// not clone the data, so if the memory is expected to be reused, the caller
// should pass a copy of the packet they wish to buffer.
func (jb *JitterBuffer) Push(packet *rtp.Packet) {
	rPacket, err := jb.packetFactory.NewPacket(&packet.Header, packet.Payload, 0, 0)
	if err != nil {
		return
	}

	// Without copying, the RetainablePacket references the caller's header and
	// payload directly, so the caller's *rtp.Packet is already a valid view.
	// Cache it so peeks/pops return it without allocating.
	if !jb.copyPackets {
		rPacket.SetUnwrapped(packet)
	}

	if jb.playbackBuffer.Length() == 0 {
		jb.emit(StartBuffering)
	}

	if !jb.reorderBuffer.Started() {
		jb.playoutHead = packet.SequenceNumber
		jb.expectedSequence = packet.SequenceNumber
	}
	jb.updateStats(packet.SequenceNumber)

	jb.reorderBuffer.Add(rPacket)
	jb.drain()

	jb.updateState()
}

func (jb *JitterBuffer) emit(event Event) {
	for _, l := range jb.listeners[event] {
		l(event, jb)
	}
}

func (jb *JitterBuffer) updateState() {
	// For now, we only look at the number of packets captured in the play buffer
	if jb.playbackBuffer.Length() >= jb.minStartCount && jb.state == Buffering {
		jb.state = Emitting
		jb.playoutReady = true
		jb.emit(BeginPlayback)
	}
}

// Peek at the packet which is either:
//
//	At the playout head when we are emitting, and the playoutHead flag is true
//
// or else
//
//	At the last sequence received
func (jb *JitterBuffer) Peek(playoutHead bool) (*rtp.Packet, error) {
	if !jb.reorderBuffer.Started() {
		return nil, ErrBufferUnderrun
	}

	var packet *rtpbuffer.RetainablePacket
	if playoutHead && jb.state == Emitting {
		packet = jb.playbackBuffer.Peek()
		if packet != nil {
			if err := packet.Retain(); err != nil {
				return nil, ErrNotFound
			}
		}
	} else {
		packet = jb.reorderBuffer.Get(jb.lastSequence)
	}

	if packet == nil {
		return nil, ErrNotFound
	}

	return jb.takePacket(packet), nil
}

// Pop an RTP packet from the jitter buffer at the current playout head.
func (jb *JitterBuffer) Pop() (*rtp.Packet, error) {
	packet, err := jb.popRetainable()
	if err != nil {
		return nil, err
	}

	return jb.takePacket(packet), nil
}

// Same as Pop, except it returns a RetainablePacket.
func (jb *JitterBuffer) popRetainable() (*rtpbuffer.RetainablePacket, error) {
	if jb.state != Emitting {
		return nil, ErrPopWhileBuffering
	}
	packet := jb.playbackBuffer.Pop()
	if packet == nil {
		jb.stats.underflowCount++
		jb.emit(BufferUnderflow)

		return nil, ErrNotFound
	}
	jb.playoutHead = packet.Header().SequenceNumber + 1
	jb.updateState()

	return packet, nil
}

// PopAtSequence will pop an RTP packet from the jitter buffer at the specified Sequence.
func (jb *JitterBuffer) PopAtSequence(sq uint16) (*rtp.Packet, error) {
	if jb.state != Emitting {
		return nil, ErrPopWhileBuffering
	}
	packet := jb.playbackBuffer.PopAt(sq)
	if packet == nil {
		jb.stats.underflowCount++
		jb.emit(BufferUnderflow)

		return nil, ErrNotFound
	}
	jb.playoutHead = sq + 1
	jb.updateState()

	return jb.takePacket(packet), nil
}

// PeekAtSequence will return an RTP packet from the jitter buffer at the specified Sequence
// without removing it from the buffer. Like RemoveAtSequence, it finds a packet
// still queued for playout even if newer packets pushed it out of the window.
func (jb *JitterBuffer) PeekAtSequence(sq uint16) (*rtp.Packet, error) {
	packet := jb.reorderBuffer.Peek(sq)
	if packet == nil {
		packet = jb.playbackBuffer.At(sq)
	}
	if packet == nil {
		return nil, ErrNotFound
	}

	return jb.view(packet), nil
}

// RemoveAtSequence removes the packet with the specified Sequence from the
// buffer and returns it, regardless of playout state. Playout is consumed
// through sq whether or not it was buffered: packets still queued at or before
// sq are discarded, and playout resumes with the packets after sq. A packet
// still queued for playout is returned even if newer packets have pushed it
// out of the sequence window.
func (jb *JitterBuffer) RemoveAtSequence(sq uint16) (*rtp.Packet, error) {
	packet := jb.reorderBuffer.Remove(sq)

	// sq is at or after the drain point when it is less than half the sequence
	// space ahead of it.
	if sq-jb.expectedSequence < rtpbuffer.Uint16SizeHalf {
		// Everything queued precedes the drain point and so sq. Moving the
		// drain point past sq stops drain waiting forever for sq (or a gap
		// before it) instead of queueing the packets after it.
		jb.playbackBuffer.Clear()
		jb.expectedSequence = sq + 1
		jb.drain()
		jb.updateState()
	} else {
		// sq is behind the drain point, and the queue is shorter than half the
		// sequence space, so comparing queued packets with sq is unambiguous.
		for head := jb.playbackBuffer.Peek(); head != nil; head = jb.playbackBuffer.Peek() {
			if diff := head.Header().SequenceNumber - sq; diff != 0 && diff < rtpbuffer.Uint16SizeHalf {
				break
			}
			queued := jb.playbackBuffer.Pop()
			if packet == nil && queued.Header().SequenceNumber == sq {
				// The window moved past sq and its slot was reused, but the
				// queue still holds a reference: hand that one to the caller.
				packet = queued

				continue
			}
			queued.Release()
		}
	}

	// The next packet to pop is the queue head, or the next one drain will
	// move in.
	if head := jb.playbackBuffer.Peek(); head != nil {
		jb.playoutHead = head.Header().SequenceNumber
	} else {
		jb.playoutHead = jb.expectedSequence
	}

	if packet == nil {
		return nil, ErrNotFound
	}

	return jb.takePacket(packet), nil
}

// PopAtTimestamp pops an RTP packet from the jitter buffer with the provided timestamp
// Call this method repeatedly to drain the buffer at the timestamp.
func (jb *JitterBuffer) PopAtTimestamp(ts uint32) (*rtp.Packet, error) {
	if jb.state != Emitting {
		return nil, ErrPopWhileBuffering
	}
	packet := jb.playbackBuffer.PopAtTimestamp(ts)
	if packet == nil {
		jb.stats.underflowCount++
		jb.emit(BufferUnderflow)

		return nil, ErrNotFound
	}
	jb.playoutHead = packet.Header().SequenceNumber + 1
	jb.updateState()

	return jb.takePacket(packet), nil
}

// Unwrap the packet and drop the caller's reference to it.
func (jb *JitterBuffer) takePacket(rPacket *rtpbuffer.RetainablePacket) *rtp.Packet {
	pkt := jb.view(rPacket)
	rPacket.Release()

	return pkt
}

// view returns the cached *rtp.Packet for rPacket. Without copying, Push
// caches the caller's packet. With copying, the first read builds a copy that
// does not alias the factory's pooled buffers, which are reused once the
// packet is released.
func (jb *JitterBuffer) view(rPacket *rtpbuffer.RetainablePacket) *rtp.Packet {
	if pkt := rPacket.Unwrapped(); pkt != nil {
		return pkt
	}

	// A shallow header copy is safe: the factory gives each pooled header a
	// freshly cloned header and never writes into its old CSRC/extension slices.
	pkt := &rtp.Packet{
		Header:  *rPacket.Header(),
		Payload: append([]byte(nil), rPacket.Payload()...),
	}
	rPacket.SetUnwrapped(pkt)

	return pkt
}

// Move the packets into the playback buffer as long as they are in order.
func (jb *JitterBuffer) drain() {
	for range jb.overflowLen {
		rPacket := jb.reorderBuffer.Get(jb.expectedSequence)
		if rPacket == nil {
			break
		}
		if !jb.playbackBuffer.Push(rPacket) {
			rPacket.Release()
			jb.stats.overflowCount++
			jb.emit(BufferOverflow)

			break
		}
		jb.expectedSequence++
	}
}

// Clear will empty the buffer and optionally reset the state. Configuration
// such as the minimum packet count is kept.
func (jb *JitterBuffer) Clear(resetState bool) {
	jb.reorderBuffer.Clear()
	jb.playbackBuffer.Clear()

	if resetState {
		jb.lastSequence = 0
		jb.expectedSequence = 0
		jb.playoutHead = 0
		jb.playoutReady = false
		jb.state = Buffering
		jb.stats = Stats{0, 0, 0}
	}
}
