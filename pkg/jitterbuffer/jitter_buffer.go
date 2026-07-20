// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package jitterbuffer implements a buffer for RTP packets designed to help
// counteract non-deterministic sources of latency
package jitterbuffer

import (
	"errors"
	"sync"

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
type JitterBuffer struct {
	packetFactory    rtpbuffer.PacketFactory
	reorderBuffer    *rtpbuffer.RTPBuffer
	playbackBuffer   *RingBuffer
	minStartCount    uint16
	overflowLen      uint16
	lastSequence     uint16
	expectedSequence uint16
	playoutHead      uint16
	playoutReady     bool
	state            State
	stats            Stats
	listeners        map[Event][]EventListener
	mutex            sync.Mutex
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

	jb.packetFactory = rtpbuffer.NewPacketFactoryCopy()
	jb.reorderBuffer, _ = rtpbuffer.NewRTPBuffer(jb.overflowLen)
	jb.playbackBuffer = NewRingBuffer(jb.overflowLen)

	return jb
}

// WithMinimumPacketCount will set the required number of packets to be received before
// any attempt to pop a packet can succeed.
func WithMinimumPacketCount(count uint16) Option {
	return func(jb *JitterBuffer) {
		jb.minStartCount = count
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
	jb.mutex.Lock()
	defer jb.mutex.Unlock()

	return jb.playoutHead
}

// SetPlayoutHead allows you to manually specify the packet you wish to pop next
// If you have encountered a packet that hasn't resolved you can skip it.
func (jb *JitterBuffer) SetPlayoutHead(playoutHead uint16) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()

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

// Push an RTP packet into the jitter buffer, this does not clone
// the data so if the memory is expected to be reused, the caller should
// take this in to account and pass a copy of the packet they wish to buffer.
func (jb *JitterBuffer) Push(packet *rtp.Packet) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()

	rPacket, err := jb.packetFactory.NewPacket(&packet.Header, packet.Payload, 0, 0)
	if err != nil {
		return
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
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
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

	return takePacket(packet), nil
}

// Pop an RTP packet from the jitter buffer at the current playout head.
func (jb *JitterBuffer) Pop() (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	if jb.state != Emitting {
		return nil, ErrPopWhileBuffering
	}
	packet := jb.playbackBuffer.Pop()
	if packet == nil {
		jb.stats.underflowCount++
		jb.emit(BufferUnderflow)

		return nil, ErrNotFound
	}
	jb.playoutHead = (jb.playoutHead + 1)
	jb.updateState()

	return takePacket(packet), nil
}

// PopAtSequence will pop an RTP packet from the jitter buffer at the specified Sequence.
func (jb *JitterBuffer) PopAtSequence(sq uint16) (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
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

	return takePacket(packet), nil
}

// PeekAtSequence will return an RTP packet from the jitter buffer at the specified Sequence
// without removing it from the buffer.
func (jb *JitterBuffer) PeekAtSequence(sq uint16) (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	packet := jb.reorderBuffer.Get(sq)
	if packet == nil {
		return nil, ErrNotFound
	}

	return takePacket(packet), nil
}

// PopAtTimestamp pops an RTP packet from the jitter buffer with the provided timestamp
// Call this method repeatedly to drain the buffer at the timestamp.
func (jb *JitterBuffer) PopAtTimestamp(ts uint32) (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
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

	return takePacket(packet), nil
}

// Unwrap the packet.
func takePacket(rPacket *rtpbuffer.RetainablePacket) *rtp.Packet {
	payload := rPacket.Payload()
	out := make([]byte, len(payload))
	copy(out, payload)
	hdr := *rPacket.Header()
	rPacket.Release()

	return &rtp.Packet{
		Header:  hdr,
		Payload: out,
	}
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

// Clear will empty the buffer and optionally reset the state.
func (jb *JitterBuffer) Clear(resetState bool) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	jb.reorderBuffer.Clear()
	jb.playbackBuffer.Clear()

	if resetState {
		jb.lastSequence = 0
		jb.expectedSequence = 0
		jb.playoutHead = 0
		jb.playoutReady = false
		jb.state = Buffering
		jb.stats = Stats{0, 0, 0}
		jb.minStartCount = 50
	}
}
