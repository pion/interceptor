// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package jitterbuffer implements a buffer for RTP packets designed to help
// counteract non-deterministic sources of latency
package jitterbuffer

import (
	"errors"
	"math"
	"sync"

	"github.com/pion/rtp"
)

// State tracks a JitterBuffer as either Buffering or Emitting
type State uint16

// Event represents all events a JitterBuffer can emit
type Event string

var (
	// ErrBufferUnderrun is returned when the buffer has no items
	ErrBufferUnderrun = errors.New("invalid Peek: Empty jitter buffer")
	// ErrPopWhileBuffering is returned if a jitter buffer is not in a playback state
	ErrPopWhileBuffering = errors.New("attempt to pop while buffering")
)

const (
	// Buffering is the state when the jitter buffer has not started emitting yet, or has hit an underflow and needs to re-buffer packets
	Buffering State = iota
	//  Emitting is the state when the jitter buffer is operating nominally
	Emitting
)

const (
	// StartBuffering is emitted when the buffer receives its first packet
	StartBuffering Event = "startBuffering"
	// BeginPlayback is emitted when the buffer has satisfied its buffer length
	BeginPlayback = "playing"
	// BufferUnderflow is emitted when the buffer does not have enough packets to Pop
	BufferUnderflow = "underflow"
	// BufferOverflow is emitted when the buffer has exceeded its limit
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
	// Option will Override JitterBuffer's defaults
	Option func(jb *JitterBuffer)
	// EventListener will be called when the corresponding Event occurs
	EventListener func(event Event, jb *JitterBuffer)
)

// A JitterBuffer will accept Pushed packets, put them in sequence number
// order, and allows removing in either sequence number order or via a
// provided timestamp
type JitterBuffer struct {
	packets       *PriorityQueue
	minStartCount uint16
	lastSequence  uint16
	playoutHead   uint16
	playoutReady  bool
	state         State
	stats         Stats
	listeners     map[Event][]EventListener
	mutex         sync.Mutex
}

// Stats Track interesting statistics for the life of this JitterBuffer
// outOfOrderCount will provide the number of times a packet was Pushed
//
//	without its predecessor being present
//
// underflowCount will provide the count of attempts to Pop an empty buffer
// overflowCount will track the number of times the jitter buffer exceeds its limit
type Stats struct {
	outOfOrderCount uint32
	underflowCount  uint32
	overflowCount   uint32
}

// New will initialize a jitter buffer and its associated statistics
func New(opts ...Option) *JitterBuffer {
	jb := &JitterBuffer{state: Buffering, stats: Stats{0, 0, 0}, minStartCount: 50, packets: NewQueue(), listeners: make(map[Event][]EventListener)}
	for _, o := range opts {
		o(jb)
	}
	return jb
}

// WithMinimumPacketCount will set the required number of packets to be received before
// any attempt to pop a packet can succeed
func WithMinimumPacketCount(count uint16) Option {
	return func(jb *JitterBuffer) {
		jb.minStartCount = count
	}
}

// Listen will register an event listener
// The jitter buffer may emit events correspnding, interested listerns should
// look at Event for available events
func (jb *JitterBuffer) Listen(event Event, cb EventListener) {
	jb.listeners[event] = append(jb.listeners[event], cb)
}

func (jb *JitterBuffer) updateStats(lastPktSeqNo uint16) {
	// If we have at least one packet, and the next packet being pushed in is not
	// at the expected sequence number increment the out of order count
	if jb.packets.Length() > 0 && lastPktSeqNo != ((jb.lastSequence+1)%math.MaxUint16) {
		jb.stats.outOfOrderCount++
	}
	jb.lastSequence = lastPktSeqNo
}

// Push an RTP packet into the jitter buffer, this does not clone
// the data so if the memory is expected to be reused, the caller should
// take this in to account and pass a copy of the packet they wish to buffer
func (jb *JitterBuffer) Push(packet *rtp.Packet) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	if jb.packets.Length() == 0 {
		jb.emit(StartBuffering)
	}
	if jb.packets.Length() > 100 {
		jb.stats.overflowCount++
		jb.emit(BufferOverflow)
	}
	if !jb.playoutReady && jb.packets.Length() == 0 {
		jb.playoutHead = packet.SequenceNumber
	}
	jb.updateStats(packet.SequenceNumber)
	jb.packets.Push(packet, packet.SequenceNumber)
	jb.updateState()
}

func (jb *JitterBuffer) emit(event Event) {
	for _, l := range jb.listeners[event] {
		l(event, jb)
	}
}

func (jb *JitterBuffer) updateState() {
	// For now, we only look at the number of packets captured in the play buffer
	if jb.packets.Length() >= jb.minStartCount && jb.state == Buffering {
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
	if jb.packets.Length() < 1 {
		return nil, ErrBufferUnderrun
	}
	if playoutHead && jb.state == Emitting {
		return jb.packets.Find(jb.playoutHead)
	}
	return jb.packets.Find(jb.lastSequence)
}

// Pop an RTP packet from the jitter buffer at the current playout head
func (jb *JitterBuffer) Pop() (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	if jb.state != Emitting {
		return nil, ErrPopWhileBuffering
	}
	packet, err := jb.packets.PopAt(jb.playoutHead)
	if err != nil {
		jb.stats.underflowCount++
		jb.emit(BufferUnderflow)
		return (*rtp.Packet)(nil), err
	}
	jb.playoutHead = (jb.playoutHead + 1) % math.MaxUint16
	jb.updateState()
	return packet, nil
}

// PopAtSequence will pop an RTP packet from the jitter buffer at the specified Sequence
func (jb *JitterBuffer) PopAtSequence(sq uint16) (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	if jb.state != Emitting {
		return nil, ErrPopWhileBuffering
	}
	packet, err := jb.packets.PopAt(sq)
	if err != nil {
		jb.stats.underflowCount++
		jb.emit(BufferUnderflow)
		return (*rtp.Packet)(nil), err
	}
	jb.playoutHead = (jb.playoutHead + 1) % math.MaxUint16
	jb.updateState()
	return packet, nil
}

// PeekAtSequence will return an RTP packet from the jitter buffer at the specified Sequence
// without removing it from the buffer
func (jb *JitterBuffer) PeekAtSequence(sq uint16) (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	packet, err := jb.packets.Find(sq)
	if err != nil {
		return (*rtp.Packet)(nil), err
	}
	return packet, nil
}

// PopAtTimestamp pops an RTP packet from the jitter buffer with the provided timestamp
// Call this method repeatedly to drain the buffer at the timestamp
func (jb *JitterBuffer) PopAtTimestamp(ts uint32) (*rtp.Packet, error) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	if jb.state != Emitting {
		return nil, ErrPopWhileBuffering
	}
	packet, err := jb.packets.PopAtTimestamp(ts)
	if err != nil {
		jb.stats.underflowCount++
		jb.emit(BufferUnderflow)
		return (*rtp.Packet)(nil), err
	}
	jb.updateState()
	return packet, nil
}
