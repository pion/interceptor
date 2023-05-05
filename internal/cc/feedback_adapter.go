// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package cc

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// TwccExtensionAttributesKey identifies the TWCC value in the attribute collection
// so we don't need to reparse
const TwccExtensionAttributesKey = iota

var (
	errMissingTWCCExtension = errors.New("missing transport layer cc header extension")
	errInvalidFeedback      = errors.New("invalid feedback")
)

// FeedbackAdapter converts incoming RTCP Packets (TWCC and RFC8888) into Acknowledgments.
// Acknowledgments are the common format that Congestion Controllers in Pion understand.
type FeedbackAdapter struct {
	lock    sync.Mutex
	history *feedbackHistory
}

// NewFeedbackAdapter returns a new FeedbackAdapter
func NewFeedbackAdapter() *FeedbackAdapter {
	return &FeedbackAdapter{history: newFeedbackHistory(250)}
}

func (f *FeedbackAdapter) onSentRFC8888(ts time.Time, header *rtp.Header, size int) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.history.add(Acknowledgment{
		SequenceNumber: header.SequenceNumber,
		SSRC:           header.SSRC,
		Size:           size,
		Departure:      ts,
		Arrival:        time.Time{},
		ECN:            0,
	})
	return nil
}

func (f *FeedbackAdapter) onSentTWCC(ts time.Time, extID uint8, header *rtp.Header, size int) error {
	sequenceNumber := header.GetExtension(extID)
	var tccExt rtp.TransportCCExtension
	err := tccExt.Unmarshal(sequenceNumber)
	if err != nil {
		return errMissingTWCCExtension
	}

	f.lock.Lock()
	defer f.lock.Unlock()
	f.history.add(Acknowledgment{
		SequenceNumber: tccExt.TransportSequence,
		SSRC:           0,
		Size:           header.MarshalSize() + size,
		Departure:      ts,
		Arrival:        time.Time{},
		ECN:            0,
	})
	return nil
}

// OnSent records that and when an outgoing packet was sent for later mapping to
// acknowledgments
func (f *FeedbackAdapter) OnSent(ts time.Time, header *rtp.Header, size int, attributes interceptor.Attributes) error {
	hdrExtensionID := attributes.Get(TwccExtensionAttributesKey)
	id, ok := hdrExtensionID.(uint8)
	if ok && hdrExtensionID != 0 {
		return f.onSentTWCC(ts, id, header, size)
	}

	return f.onSentRFC8888(ts, header, size)
}

func (f *FeedbackAdapter) unpackRunLengthChunk(start uint16, refTime time.Time, chunk *rtcp.RunLengthChunk, deltas []*rtcp.RecvDelta) (consumedDeltas int, nextRef time.Time, acks []Acknowledgment, err error) {
	result := make([]Acknowledgment, chunk.RunLength)
	deltaIndex := 0

	end := start + chunk.RunLength
	resultIndex := 0
	for i := start; i != end; i++ {
		key := feedbackHistoryKey{
			ssrc:           0,
			sequenceNumber: i,
		}
		if ack, ok := f.history.get(key); ok {
			if chunk.PacketStatusSymbol != rtcp.TypeTCCPacketNotReceived {
				if len(deltas)-1 < deltaIndex {
					return deltaIndex, refTime, result, errInvalidFeedback
				}
				refTime = refTime.Add(time.Duration(deltas[deltaIndex].Delta) * time.Microsecond)
				ack.Arrival = refTime
				deltaIndex++
			}
			result[resultIndex] = ack
		}
		resultIndex++
	}
	return deltaIndex, refTime, result, nil
}

func (f *FeedbackAdapter) unpackStatusVectorChunk(start uint16, refTime time.Time, chunk *rtcp.StatusVectorChunk, deltas []*rtcp.RecvDelta) (consumedDeltas int, nextRef time.Time, acks []Acknowledgment, err error) {
	result := make([]Acknowledgment, len(chunk.SymbolList))
	deltaIndex := 0
	resultIndex := 0
	for i, symbol := range chunk.SymbolList {
		key := feedbackHistoryKey{
			ssrc:           0,
			sequenceNumber: start + uint16(i),
		}
		if ack, ok := f.history.get(key); ok {
			if symbol != rtcp.TypeTCCPacketNotReceived {
				if len(deltas)-1 < deltaIndex {
					return deltaIndex, refTime, result, errInvalidFeedback
				}
				refTime = refTime.Add(time.Duration(deltas[deltaIndex].Delta) * time.Microsecond)
				ack.Arrival = refTime
				deltaIndex++
			}
			result[resultIndex] = ack
		}
		resultIndex++
	}

	return deltaIndex, refTime, result, nil
}

// OnTransportCCFeedback converts incoming TWCC RTCP packet feedback to
// Acknowledgments.
func (f *FeedbackAdapter) OnTransportCCFeedback(_ time.Time, feedback *rtcp.TransportLayerCC) ([]Acknowledgment, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	result := []Acknowledgment{}
	index := feedback.BaseSequenceNumber
	refTime := time.Time{}.Add(time.Duration(feedback.ReferenceTime) * 64 * time.Millisecond)
	recvDeltas := feedback.RecvDeltas

	for _, chunk := range feedback.PacketChunks {
		switch chunk := chunk.(type) {
		case *rtcp.RunLengthChunk:
			n, nextRefTime, acks, err := f.unpackRunLengthChunk(index, refTime, chunk, recvDeltas)
			if err != nil {
				return nil, err
			}
			refTime = nextRefTime
			result = append(result, acks...)
			recvDeltas = recvDeltas[n:]
			index = uint16(int(index) + len(acks))
		case *rtcp.StatusVectorChunk:
			n, nextRefTime, acks, err := f.unpackStatusVectorChunk(index, refTime, chunk, recvDeltas)
			if err != nil {
				return nil, err
			}
			refTime = nextRefTime
			result = append(result, acks...)
			recvDeltas = recvDeltas[n:]
			index = uint16(int(index) + len(acks))
		default:
			return nil, errInvalidFeedback
		}
	}

	return result, nil
}

// OnRFC8888Feedback converts incoming Congestion Control Feedback RTCP packet
// to Acknowledgments.
func (f *FeedbackAdapter) OnRFC8888Feedback(_ time.Time, feedback *rtcp.CCFeedbackReport) []Acknowledgment {
	f.lock.Lock()
	defer f.lock.Unlock()

	result := []Acknowledgment{}
	referenceTime := ntp.ToTime(uint64(feedback.ReportTimestamp) << 16)
	for _, rb := range feedback.ReportBlocks {
		for i, mb := range rb.MetricBlocks {
			sequenceNumber := rb.BeginSequence + uint16(i)
			key := feedbackHistoryKey{
				ssrc:           rb.MediaSSRC,
				sequenceNumber: sequenceNumber,
			}
			if ack, ok := f.history.get(key); ok {
				if mb.Received {
					delta := time.Duration((float64(mb.ArrivalTimeOffset) / 1024.0) * float64(time.Second))
					ack.Arrival = referenceTime.Add(-delta)
					ack.ECN = mb.ECN
				}
				result = append(result, ack)
			}
		}
	}
	return result
}

type feedbackHistoryKey struct {
	ssrc           uint32
	sequenceNumber uint16
}

type feedbackHistory struct {
	size      int
	evictList *list.List
	items     map[feedbackHistoryKey]*list.Element
}

func newFeedbackHistory(size int) *feedbackHistory {
	return &feedbackHistory{
		size:      size,
		evictList: list.New(),
		items:     make(map[feedbackHistoryKey]*list.Element),
	}
}

func (f *feedbackHistory) get(key feedbackHistoryKey) (Acknowledgment, bool) {
	ent, ok := f.items[key]
	if ok {
		if ack, ok := ent.Value.(Acknowledgment); ok {
			return ack, true
		}
	}
	return Acknowledgment{}, false
}

func (f *feedbackHistory) add(ack Acknowledgment) {
	key := feedbackHistoryKey{
		ssrc:           ack.SSRC,
		sequenceNumber: ack.SequenceNumber,
	}
	// Check for existing
	if ent, ok := f.items[key]; ok {
		f.evictList.MoveToFront(ent)
		ent.Value = ack
		return
	}
	// Add new
	ent := f.evictList.PushFront(ack)
	f.items[key] = ent
	// Evict if necessary
	if f.evictList.Len() > f.size {
		f.removeOldest()
	}
}

func (f *feedbackHistory) removeOldest() {
	if ent := f.evictList.Back(); ent != nil {
		f.evictList.Remove(ent)
		if ack, ok := ent.Value.(Acknowledgment); ok {
			key := feedbackHistoryKey{
				ssrc:           ack.SSRC,
				sequenceNumber: ack.SequenceNumber,
			}
			delete(f.items, key)
		}
	}
}
