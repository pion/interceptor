package gcc

import (
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const transportCCURI = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"

// Pacer is the interface implemented by packet pacers
type Pacer interface {
	interceptor.RTPWriter
	AddStream(ssrc uint32, writer interceptor.RTPWriter)
	SetTargetBitrate(int)
	Close() error
}

// Stats contains internal statistics of the bandwidth estimator
type Stats struct {
	LossStats
	DelayStats
}

type SendSideBWE struct {
	lastBitrate int
	Pacer
	*lossBasedBandwidthEstimator
	*delayBasedBandwidthEstimator
	*FeedbackAdapter

	onTargetBitrateChange func(bitrate int)

	stats   chan Stats
	bitrate chan int

	close chan struct{}
}

type SendSideBWEOption func(*SendSideBWE) error

// InitialBitrate sets the initial bitrate of new GCC interceptors
func SendSideBWEInitialBitrate(rate int) SendSideBWEOption {
	return func(e *SendSideBWE) error {
		e.lastBitrate = rate
		return nil
	}
}

func NewSendSideBWE(opts ...SendSideBWEOption) (*SendSideBWE, error) {
	e := &SendSideBWE{
		lastBitrate:                  100_000,
		Pacer:                        nil,
		lossBasedBandwidthEstimator:  nil,
		delayBasedBandwidthEstimator: nil,
		FeedbackAdapter:              NewFeedbackAdapter(),
		onTargetBitrateChange:        nil,
		stats:                        make(chan Stats),
		bitrate:                      make(chan int),
		close:                        make(chan struct{}),
	}
	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}
	if e.Pacer == nil {
		e.Pacer = NewLeakyBucketPacer(e.lastBitrate)
	}
	e.lossBasedBandwidthEstimator = newLossBasedBWE(e.lastBitrate)
	e.delayBasedBandwidthEstimator = newDelayBasedBWE(e.lastBitrate)

	go e.loop()

	return e, nil
}

func (e *SendSideBWE) AddStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	var hdrExtID uint8
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			hdrExtID = uint8(e.ID)
			break
		}
	}

	e.Pacer.AddStream(info.SSRC, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		if hdrExtID != 0 {
			if attributes == nil {
				attributes = make(interceptor.Attributes)
			}
			attributes.Set(twccExtensionAttributesKey, hdrExtID)
		}
		if err := e.OnSent(time.Now(), header, len(payload), attributes); err != nil {
			return 0, err
		}
		return writer.Write(header, payload, attributes)
	}))
	return e
}

func (e *SendSideBWE) WriteRTCP(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
	for _, pkt := range pkts {
		acks, err := e.OnFeedback(time.Now(), pkt)
		if err != nil {
			return 0, err
		}
		e.updateLossEstimate(acks)
		e.updateDelayEstimate(acks)
	}
	return len(pkts), nil
}

// GetTargetBitrate returns the current target bitrate
func (e *SendSideBWE) GetTargetBitrate() int {
	return <-e.bitrate
}

func (e *SendSideBWE) GetStats() map[string]interface{} {
	stats := <-e.stats
	return map[string]interface{}{
		"lossEstimate":  stats.LossStats.TargetBitrate,
		"delayEstimate": stats.DelayStats.TargetBitrate,
		"estimate":      stats.Estimate,
		"thresh":        stats.Threshold,
		"rtt":           stats.RTT.Milliseconds(),
	}
}

// OnTargetBitrateChange sets the callback that is called when the target
// bitrate changes
func (e *SendSideBWE) OnTargetBitrateChange(f func(bitrate int)) {
	e.onTargetBitrateChange = f
}

func (e *SendSideBWE) Close() error {
	close(e.close)
	return nil
}

func (e *SendSideBWE) loop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	lbrStats := e.lossBasedBandwidthEstimator.getEstimate(e.lastBitrate)
	dbrStats := e.delayBasedBandwidthEstimator.getEstimate()
	e.lastBitrate = min(dbrStats.TargetBitrate, lbrStats.TargetBitrate)
	for {
		select {
		case e.bitrate <- e.lastBitrate:
		case e.stats <- Stats{
			LossStats:  lbrStats,
			DelayStats: dbrStats,
		}:
		case <-ticker.C:
			dbrStats = e.delayBasedBandwidthEstimator.getEstimate()
			lbrStats = e.lossBasedBandwidthEstimator.getEstimate(dbrStats.TargetBitrate)

			bitrateChanged := false
			bitrate := min(dbrStats.TargetBitrate, lbrStats.TargetBitrate)
			if bitrate != e.lastBitrate {
				bitrateChanged = true
				e.lastBitrate = bitrate
				e.SetTargetBitrate(e.lastBitrate)
			}

			if bitrateChanged && e.onTargetBitrateChange != nil {
				go e.onTargetBitrateChange(bitrate)
			}
		case <-e.close:
			return
		}
	}
}
