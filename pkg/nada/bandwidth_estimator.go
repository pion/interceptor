package nada

import (
	"math"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type NadaBandwidthEstimator struct {
	*cc.FeedbackAdapter
	*Sender
	*Receiver

	onTargetBitrateChangeFunc func(int)
	lastTargetRate            int
}

func NewBandwidthEstimator() *NadaBandwidthEstimator {
	now := time.Now()
	return &NadaBandwidthEstimator{
		FeedbackAdapter: cc.NewFeedbackAdapter(),
		Sender:          NewSender(now, DefaultConfig()),
		Receiver:        NewReceiver(now, DefaultConfig()),
	}
}

func (e *NadaBandwidthEstimator) AddStream(_ *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		now := time.Now()
		if err := e.OnSent(now, header, len(payload), attributes); err != nil {
			return 0, err
		}
		return writer.Write(header, payload, attributes)
	})
}

func (e *NadaBandwidthEstimator) WriteRTCP(pkts []rtcp.Packet, _ interceptor.Attributes) error {
	now := time.Now()
	var acks []cc.Acknowledgment
	for _, pkt := range pkts {
		var feedbackSentTime time.Time
		switch fb := pkt.(type) {
		case *rtcp.TransportLayerCC:
			newAcks, err := e.OnTransportCCFeedback(now, fb)
			if err != nil {
				return err
			}
			for i, ack := range acks {
				if i == 0 {
					feedbackSentTime = ack.Arrival
					continue
				}
				if ack.Arrival.After(feedbackSentTime) {
					feedbackSentTime = ack.Arrival
				}
			}
			acks = append(acks, newAcks...)
		case *rtcp.CCFeedbackReport:
			acks = e.OnRFC8888Feedback(now, fb)
			feedbackSentTime = ntp.ToTime(uint64(fb.ReportTimestamp) << 16)
		default:
			continue
		}

		feedbackMinRTT := time.Duration(math.MaxInt)
		for _, ack := range acks {
			if ack.Arrival.IsZero() {
				continue
			}
			pendingTime := feedbackSentTime.Sub(ack.Arrival)
			rtt := now.Sub(ack.Departure) - pendingTime
			feedbackMinRTT = time.Duration(minInt(int(rtt), int(feedbackMinRTT)))
		}
		if feedbackMinRTT < math.MaxInt {
			e.UpdateEstimatedRoundTripTime(feedbackMinRTT)
		}
	}

	for _, ack := range acks {
		e.OnReceiveMediaPacket(ack.Arrival, ack.Departure, ack.SequenceNumber, ack.ECN == rtcp.ECNCE, 8*Bits(ack.Size))
	}

	e.OnReceiveFeedbackReport(now, e.BuildFeedbackReport())

	target := e.GetTargetBitrate()
	if target != e.lastTargetRate {
		e.onTargetBitrateChangeFunc(target)
		e.lastTargetRate = target
	}

	return nil
}

func (e *NadaBandwidthEstimator) GetTargetBitrate() int {
	// TODO: What is the buffer len parameter of GetTargetBitrate?
	return int(e.GetTargetRate(0))
}

func (e *NadaBandwidthEstimator) OnTargetBitrateChange(f func(bitrate int)) {
	e.onTargetBitrateChangeFunc = f
}

func (e *NadaBandwidthEstimator) GetStats() map[string]interface{} {
	panic("not implemented")
}

func (e *NadaBandwidthEstimator) Close() error {
	panic("not implemented")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
