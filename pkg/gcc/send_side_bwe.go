package gcc

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const (
	transportCCURI = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"
	latestBitrate  = 10_000
	minBitrate     = 5_000
	maxBitrate     = 50_000_000
)

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

// SendSideBWE implements a combination of loss and delay based GCC
type SendSideBWE struct {
	pacer           Pacer
	lossController  *lossBasedBandwidthEstimator
	delayController *delayController
	feedbackAdapter *cc.FeedbackAdapter

	onTargetBitrateChange func(bitrate int)

	lock          sync.Mutex
	latestStats   Stats
	latestBitrate int
	minBitrate    int
	maxBitrate    int

	close chan struct{}
}

// Option configures a bandwidth estimator
type Option func(*SendSideBWE) error

// SendSideBWEInitialBitrate sets the initial bitrate of new GCC interceptors
func SendSideBWEInitialBitrate(rate int) Option {
	return func(e *SendSideBWE) error {
		e.latestBitrate = rate
		return nil
	}
}

// SendSideBWEPacer sets the pacing algorithm to use.
func SendSideBWEPacer(p Pacer) Option {
	return func(e *SendSideBWE) error {
		e.pacer = p
		return nil
	}
}

// NewSendSideBWE creates a new sender side bandwidth estimator
func NewSendSideBWE(opts ...Option) (*SendSideBWE, error) {
	e := &SendSideBWE{
		pacer:                 nil,
		lossController:        nil,
		delayController:       nil,
		feedbackAdapter:       cc.NewFeedbackAdapter(),
		onTargetBitrateChange: nil,
		lock:                  sync.Mutex{},
		latestStats:           Stats{},
		latestBitrate:         latestBitrate,
		minBitrate:            minBitrate,
		maxBitrate:            maxBitrate,
		close:                 make(chan struct{}),
	}
	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}
	if e.pacer == nil {
		e.pacer = NewLeakyBucketPacer(e.latestBitrate)
	}
	e.lossController = newLossBasedBWE(e.latestBitrate)
	e.delayController = newDelayController(delayControllerConfig{
		nowFn:          time.Now,
		initialBitrate: e.latestBitrate,
		minBitrate:     e.minBitrate,
		maxBitrate:     e.maxBitrate,
	})

	e.delayController.onUpdate(e.onDelayUpdate)

	return e, nil
}

// AddStream adds a new stream to the bandwidth estimator
func (e *SendSideBWE) AddStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	var hdrExtID uint8
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			hdrExtID = uint8(e.ID)
			break
		}
	}

	e.pacer.AddStream(info.SSRC, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		if hdrExtID != 0 {
			if attributes == nil {
				attributes = make(interceptor.Attributes)
			}
			attributes.Set(cc.TwccExtensionAttributesKey, hdrExtID)
		}
		if err := e.feedbackAdapter.OnSent(time.Now(), header, len(payload), attributes); err != nil {
			return 0, err
		}
		return writer.Write(header, payload, attributes)
	}))
	return e.pacer
}

// WriteRTCP adds some RTCP feedback to the bandwidth estimator
func (e *SendSideBWE) WriteRTCP(pkts []rtcp.Packet, attributes interceptor.Attributes) error {
	for _, pkt := range pkts {
		acks, err := e.feedbackAdapter.OnFeedback(time.Now(), pkt)
		if err != nil {
			return err
		}
		e.lossController.updateLossEstimate(acks)
		e.delayController.updateDelayEstimate(acks)
	}
	return nil
}

// GetTargetBitrate returns the current target bitrate
func (e *SendSideBWE) GetTargetBitrate() int {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.latestBitrate
}

// GetStats returns some internal statistics of the bandwidth estimator
func (e *SendSideBWE) GetStats() map[string]interface{} {
	e.lock.Lock()
	defer e.lock.Unlock()

	return map[string]interface{}{
		"lossTargetBitrate":  e.latestStats.LossStats.TargetBitrate,
		"averageLoss":        e.latestStats.AverageLoss,
		"delayTargetBitrate": e.latestStats.DelayStats.TargetBitrate,
		"delayMeasurement":   float64(e.latestStats.Measurement.Microseconds()) / 1000.0,
		"delayEstimate":      float64(e.latestStats.Estimate.Microseconds()) / 1000.0,
		"delayThreshold":     float64(e.latestStats.Threshold.Microseconds()) / 1000.0,
		"rtt":                float64(e.latestStats.RTT.Microseconds()) / 1000.0,
		"usage":              e.latestStats.Usage.String(),
		"state":              e.latestStats.State.String(),
	}
}

// OnTargetBitrateChange sets the callback that is called when the target
// bitrate changes
func (e *SendSideBWE) OnTargetBitrateChange(f func(bitrate int)) {
	e.onTargetBitrateChange = f
}

// Close stops and closes the bandwidth estimator
func (e *SendSideBWE) Close() error {
	if err := e.delayController.Close(); err != nil {
		return err
	}
	close(e.close)
	return e.pacer.Close()
}

func (e *SendSideBWE) onDelayUpdate(delayStats DelayStats) {
	e.lock.Lock()
	defer e.lock.Unlock()

	lossStats := e.lossController.getEstimate(delayStats.TargetBitrate)
	bitrateChanged := false
	bitrate := minInt(delayStats.TargetBitrate, lossStats.TargetBitrate)
	if bitrate != e.latestBitrate {
		bitrateChanged = true
		e.latestBitrate = bitrate
		e.pacer.SetTargetBitrate(e.latestBitrate)
	}

	if bitrateChanged && e.onTargetBitrateChange != nil {
		go e.onTargetBitrateChange(bitrate)
	}

	e.latestStats = Stats{
		LossStats:  lossStats,
		DelayStats: delayStats,
	}
}
