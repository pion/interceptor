package gcc

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

var (
	errMissingTWCCExtension  = errors.New("missing transport layer cc header extension")
	errUnknownFeedbackFormat = errors.New("unknown feedback format")

	errInvalidFeedback = errors.New("invalid feedback")
)

// FeedbackAdapter converts incoming feedback from the wireformat to a
// PacketResult
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

// OnSent records when a packet was been sent.
// TODO(mathis): Is there a better way to get attributes in here?
func (f *FeedbackAdapter) OnSent(ts time.Time, header *rtp.Header, size int, attributes interceptor.Attributes) error {
	hdrExtensionID := attributes.Get(twccExtensionAttributesKey)
	id, ok := hdrExtensionID.(uint8)
	if !ok || hdrExtensionID == 0 {
		return errMissingTWCCExtension
	}
	sequenceNumber := header.GetExtension(id)
	var tccExt rtp.TransportCCExtension
	err := tccExt.Unmarshal(sequenceNumber)
	if err != nil {
		return err
	}

	f.lock.Lock()
	defer f.lock.Unlock()
	f.history[tccExt.TransportSequence] = Acknowledgment{
		TLCC:      tccExt.TransportSequence,
		Header:    header,
		Size:      size,
		Departure: ts,
		Arrival:   time.Time{},
		RTT:       0,
	}
	return nil
}

// OnFeedback converts incoming RTCP packet feedback to Acknowledgments.
// Currently only TWCC is supported.
func (f *FeedbackAdapter) OnFeedback(ts time.Time, feedback rtcp.Packet) ([]Acknowledgment, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	switch fb := feedback.(type) {
	case *rtcp.TransportLayerCC:
		return f.onIncomingTransportCC(ts, fb)
	default:
		return nil, errUnknownFeedbackFormat
	}
}

func (f *FeedbackAdapter) unpackRunLengthChunk(ts time.Time, start uint16, refTime time.Time, chunk *rtcp.RunLengthChunk, deltas []*rtcp.RecvDelta) (consumedDeltas int, nextRef time.Time, acks []Acknowledgment, err error) {
	result := make([]Acknowledgment, chunk.RunLength)
	deltaIndex := 0

	// Rollover if necessary
	end := int(start + chunk.RunLength)
	if end < int(start) {
		end += 65536
	}
	resultIndex := 0
	for i := int(start); i < end; i++ {
		if ack, ok := f.history[uint16(i)]; ok {
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

func (f *FeedbackAdapter) onIncomingTransportCC(ts time.Time, feedback *rtcp.TransportLayerCC) ([]Acknowledgment, error) {
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

// OnIncomingRFC8888 converts the incoming RFC8888 packet to a []PacketResult
func (f *FeedbackAdapter) OnIncomingRFC8888(feedback *rtcp.RawPacket) ([]Acknowledgment, error) {
	return nil, nil
}
