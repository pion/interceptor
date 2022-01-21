package cc

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// TwccExtensionAttributesKey identifies the TWCC value in the attribute collection
// so we don't need to reparse
const TwccExtensionAttributesKey = iota

var (
	errMissingTWCCExtensionID = errors.New("missing transport layer cc header extension id")
	errMissingTWCCExtension   = errors.New("missing transport layer cc header extension")
	errInvalidFeedback        = errors.New("invalid feedback")
)

// FeedbackAdapter converts incoming RTCP Packets (TWCC and RFC8888) into Acknowledgments.
// Acknowledgments are the common format that Congestion Controllers in Pion understand.
type FeedbackAdapter struct {
	lock    sync.Mutex
	history map[uint16]Acknowledgment
}

// NewFeedbackAdapter returns a new FeedbackAdapter
func NewFeedbackAdapter() *FeedbackAdapter {
	return &FeedbackAdapter{
		history: make(map[uint16]Acknowledgment),
	}
}

// OnSent records that and when an outgoing packet was sent for later mapping to
// acknowledgments
func (f *FeedbackAdapter) OnSent(ts time.Time, header *rtp.Header, size int, attributes interceptor.Attributes) error {
	hdrExtensionID := attributes.Get(TwccExtensionAttributesKey)
	id, ok := hdrExtensionID.(uint8)
	if !ok || hdrExtensionID == 0 {
		return errMissingTWCCExtensionID
	}
	sequenceNumber := header.GetExtension(id)
	var tccExt rtp.TransportCCExtension
	err := tccExt.Unmarshal(sequenceNumber)
	if err != nil {
		return errMissingTWCCExtension
	}

	f.lock.Lock()
	defer f.lock.Unlock()
	f.history[tccExt.TransportSequence] = Acknowledgment{
		TLCC:      tccExt.TransportSequence,
		Size:      header.MarshalSize() + size,
		Departure: ts,
		Arrival:   time.Time{},
		RTT:       0,
	}
	return nil
}

func (f *FeedbackAdapter) unpackRunLengthChunk(ts time.Time, start uint16, refTime time.Time, chunk *rtcp.RunLengthChunk, deltas []*rtcp.RecvDelta) (consumedDeltas int, nextRef time.Time, acks []Acknowledgment, err error) {
	result := make([]Acknowledgment, chunk.RunLength)
	deltaIndex := 0

	end := start + chunk.RunLength
	resultIndex := 0
	for i := start; i != end; i++ {
		if ack, ok := f.history[i]; ok {
			if chunk.PacketStatusSymbol != rtcp.TypeTCCPacketNotReceived {
				if len(deltas)-1 < deltaIndex {
					return deltaIndex, refTime, result, errInvalidFeedback
				}
				refTime = refTime.Add(time.Duration(deltas[deltaIndex].Delta) * time.Microsecond)
				ack.Arrival = refTime
				ack.RTT = ts.Sub(ack.Departure)
				deltaIndex++
			}
			result[resultIndex] = ack
		}
		resultIndex++
	}
	return deltaIndex, refTime, result, nil
}

func (f *FeedbackAdapter) unpackStatusVectorChunk(ts time.Time, start uint16, refTime time.Time, chunk *rtcp.StatusVectorChunk, deltas []*rtcp.RecvDelta) (consumedDeltas int, nextRef time.Time, acks []Acknowledgment, err error) {
	result := make([]Acknowledgment, len(chunk.SymbolList))
	deltaIndex := 0
	resultIndex := 0
	for i, symbol := range chunk.SymbolList {
		if ack, ok := f.history[start+uint16(i)]; ok {
			if symbol != rtcp.TypeTCCPacketNotReceived {
				if len(deltas)-1 < deltaIndex {
					return deltaIndex, refTime, result, errInvalidFeedback
				}
				refTime = refTime.Add(time.Duration(deltas[deltaIndex].Delta) * time.Microsecond)
				ack.Arrival = refTime
				ack.RTT = ts.Sub(ack.Departure)
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
func (f *FeedbackAdapter) OnTransportCCFeedback(ts time.Time, feedback *rtcp.TransportLayerCC) ([]Acknowledgment, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	result := []Acknowledgment{}
	index := feedback.BaseSequenceNumber
	refTime := time.Time{}.Add(time.Duration(feedback.ReferenceTime) * 64 * time.Millisecond)
	recvDeltas := feedback.RecvDeltas

	for _, chunk := range feedback.PacketChunks {
		switch chunk := chunk.(type) {
		case *rtcp.RunLengthChunk:
			n, nextRefTime, acks, err := f.unpackRunLengthChunk(ts, index, refTime, chunk, recvDeltas)
			if err != nil {
				return nil, err
			}
			refTime = nextRefTime
			result = append(result, acks...)
			recvDeltas = recvDeltas[n:]
			index = uint16(int(index) + len(acks))
		case *rtcp.StatusVectorChunk:
			n, nextRefTime, acks, err := f.unpackStatusVectorChunk(ts, index, refTime, chunk, recvDeltas)
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
