package gcc

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

var errMissingTWCCExtension = errors.New("missing transport layer cc header extension")
var errUnknownFeedbackFormat = errors.New("unknown feedback format")

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
		Header:    header,
		Size:      size,
		Departure: ts,
		Arrival:   time.Time{},
	}
	return nil
}

func (f *FeedbackAdapter) OnFeedback(feedback rtcp.Packet) ([]Acknowledgment, error) {
	switch fb := feedback.(type) {
	case *rtcp.TransportLayerCC:
		return f.OnIncomingTransportCC(fb)
	default:
		return nil, errUnknownFeedbackFormat
	}
}

// OnIncomingTransportCC converts the incoming rtcp.TransportLayerCC to a
// []PacketResult
func (f *FeedbackAdapter) OnIncomingTransportCC(feedback *rtcp.TransportLayerCC) ([]Acknowledgment, error) {
	t0 := time.Now()
	f.lock.Lock()
	defer f.lock.Unlock()

	result := []Acknowledgment{}

	packetStatusCount := uint16(0)
	chunkIndex := 0
	deltaIndex := 0
	referenceTime := time.Time{}.Add(time.Duration(feedback.ReferenceTime) * 64 * time.Millisecond)

	for packetStatusCount < feedback.PacketStatusCount {
		if chunkIndex >= len(feedback.PacketChunks) || len(feedback.PacketChunks) == 0 {
			return nil, errUnknownFeedbackFormat
		}
		switch packetChunk := feedback.PacketChunks[chunkIndex].(type) {
		case *rtcp.RunLengthChunk:
			symbol := packetChunk.PacketStatusSymbol
			for i := uint16(0); i < packetChunk.RunLength; i++ {
				if sentPacket, ok := f.history[feedback.BaseSequenceNumber+packetStatusCount]; ok {
					if symbol == rtcp.TypeTCCPacketReceivedSmallDelta ||
						symbol == rtcp.TypeTCCPacketReceivedLargeDelta {
						if deltaIndex >= len(feedback.RecvDeltas) {
							// TODO(mathis): Not enough recv deltas for number
							// of received packets: warn or error?
							continue
						}
						receiveTime := referenceTime.Add(time.Duration(feedback.RecvDeltas[deltaIndex].Delta) * time.Microsecond)
						referenceTime = receiveTime
						sentPacket.Arrival = receiveTime
						sentPacket.RTT = t0.Sub(sentPacket.Departure)
						result = append(result, sentPacket)
						deltaIndex++
					} else {
						result = append(result, sentPacket)
					}
				} else {
					// TODO(mathis): got feedback for unsent packet?
				}
				packetStatusCount++
			}
			chunkIndex++
		case *rtcp.StatusVectorChunk:
			for _, symbol := range packetChunk.SymbolList {
				if sentPacket, ok := f.history[feedback.BaseSequenceNumber+packetStatusCount]; ok {
					if symbol == rtcp.TypeTCCPacketReceivedSmallDelta ||
						symbol == rtcp.TypeTCCPacketReceivedLargeDelta {
						if deltaIndex >= len(feedback.RecvDeltas) {
							// TODO(mathis): Not enough recv deltas for number
							// of received packets: warn or error?
							continue
						}
						receiveTime := referenceTime.Add(time.Duration(feedback.RecvDeltas[deltaIndex].Delta) * time.Microsecond)
						referenceTime = receiveTime
						sentPacket.Arrival = receiveTime
						sentPacket.RTT = t0.Sub(sentPacket.Departure)
						result = append(result, sentPacket)
						deltaIndex++
					} else {
						result = append(result, sentPacket)
					}
				}
				packetStatusCount++
				if packetStatusCount >= feedback.PacketStatusCount {
					break
				}
			}
			chunkIndex++
		}
	}
	return result, nil
}

// OnIncomingRFC8888 converts the incoming RFC8888 packet to a []PacketResult
func (f *FeedbackAdapter) OnIncomingRFC8888(feedback *rtcp.RawPacket) ([]Acknowledgment, error) {
	return nil, nil
}

func sortedKeysUint16(m map[uint16]Acknowledgment) []uint16 {
	var result []uint16
	for k := range m {
		result = append(result, k)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}
