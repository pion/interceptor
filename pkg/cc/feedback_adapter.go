package cc

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/types"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

var errMissingTWCCExtension = errors.New("missing transport layer cc header extension")
var errInvalidFeedbackPacket = errors.New("got invalid feedback packet")

// TODO(mathis): make types internal only?

// FeedbackAdapter converts incoming feedback from the wireformat to a
// PacketResult
type FeedbackAdapter struct {
	lock    sync.Mutex
	history map[uint16]types.SentPacket
}

// NewFeedbackAdapter returns a new FeedbackAdapter
func NewFeedbackAdapter() *FeedbackAdapter {
	return &FeedbackAdapter{
		history: make(map[uint16]types.SentPacket),
	}
}

// OnSent records when a packet was been sent.
// TODO(mathis): Is there a better way to get attributes in here?
func (f *FeedbackAdapter) OnSent(ts time.Time, header *rtp.Header, attributes interceptor.Attributes) error {
	hdrExtensionID := attributes.Get(twccExtension)
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
	f.history[tccExt.TransportSequence] = types.SentPacket{
		SendTime: ts,
		Header:   header,
	}
	return nil
}

// OnIncomingTransportCC converts the incoming rtcp.TransportLayerCC to a
// []PacketResult
func (f *FeedbackAdapter) OnIncomingTransportCC(feedback *rtcp.TransportLayerCC) ([]types.PacketResult, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	result := []types.PacketResult{}

	packetStatusCount := uint16(0)
	chunkIndex := 0
	deltaIndex := 0
	referenceTime := time.Time{}.Add(time.Duration(feedback.ReferenceTime) * 64 * time.Millisecond)

	for packetStatusCount < feedback.PacketStatusCount {
		if chunkIndex >= len(feedback.PacketChunks) || len(feedback.PacketChunks) == 0 {
			return nil, errInvalidFeedbackPacket
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
						receiveTime := getReceiveTime(referenceTime, feedback.RecvDeltas[deltaIndex])
						referenceTime = receiveTime
						result = append(result, types.PacketResult{
							SentPacket:  sentPacket,
							ReceiveTime: receiveTime,
							Received:    true,
						})
						deltaIndex++
					} else {
						result = append(result, types.PacketResult{
							SentPacket:  sentPacket,
							ReceiveTime: time.Time{},
							Received:    false,
						})
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
						receiveTime := getReceiveTime(referenceTime, feedback.RecvDeltas[deltaIndex])
						referenceTime = receiveTime
						result = append(result, types.PacketResult{
							SentPacket:  sentPacket,
							ReceiveTime: receiveTime,
							Received:    true,
						})
						deltaIndex++
					} else {
						result = append(result, types.PacketResult{
							SentPacket:  sentPacket,
							ReceiveTime: time.Time{},
							Received:    false,
						})
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
func (f *FeedbackAdapter) OnIncomingRFC8888(feedback *rtcp.RawPacket) ([]types.PacketResult, error) {
	return nil, nil
}

func sortedKeysUint16(m map[uint16]types.SentPacket) []uint16 {
	var result []uint16
	for k := range m {
		result = append(result, k)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func getReceiveTime(baseTime time.Time, delta *rtcp.RecvDelta) time.Time {
	if delta.Type == rtcp.TypeTCCPacketReceivedSmallDelta {
		return baseTime.Add(time.Duration(delta.Delta) * 250 * time.Microsecond)
	}
	return baseTime.Add(time.Duration(delta.Delta) * time.Millisecond)
}
