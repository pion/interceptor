// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/pion/bwe/gcc"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/interceptor/pkg/rtpfb"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const (
	transportCCURI = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"
	latestBitrate  = 10_000
	minBitrate     = 5_000
	maxBitrate     = 50_000_000
)

// ErrSendSideBWEClosed is raised when SendSideBWE.WriteRTCP is called after SendSideBWE.Close.
var ErrSendSideBWEClosed = errors.New("SendSideBwe closed")

// Pacer is the interface implemented by packet pacers.
type Pacer interface {
	interceptor.RTPWriter
	AddStream(ssrc uint32, writer interceptor.RTPWriter)
	SetTargetBitrate(int)
	Close() error
}

// Stats contains internal statistics of the bandwidth estimator.
//
// Deprecated: All stats will always be zero.
type Stats struct {
	LossStats
	DelayStats
}

// LossStats contains internal statistics of the loss based controller.
//
// Deprecated: All stats will always be zero.
type LossStats struct {
	TargetBitrate int
	AverageLoss   float64
}

// DelayStats contains some internal statistics of the delay based congestion
// controller.
//
// Deprecated: All stats will always be zero.
type DelayStats struct {
	Measurement      time.Duration
	Estimate         time.Duration
	Threshold        time.Duration
	LastReceiveDelta time.Duration

	Usage         usage
	State         state
	TargetBitrate int
}

// Deprecated but necessary to not break the stats API.
type state int

// Deprecated but necessary to not break the stats API.
func (s state) String() string {
	return "state is deprecated"
}

// Deprecated but necessary to not break the stats API.
type usage int

// Deprecated but necessary to not break the stats API.
func (u usage) String() string {
	return "usage is deprecated"
}

// SendSideBWE implements a combination of loss and delay based GCC.
//
// Deprecated: SendSideBWE is now a wrapper around the new GCC implementation
// from https://github.com/pion/bwe
// New applications should directly use that implementation without depending on
// the legacy API from this package.
type SendSideBWE struct {
	pacer                 Pacer
	latestBitrate         atomic.Int64
	bwe                   *gcc.SendSideController
	loggerFactory         logging.LoggerFactory
	onTargetBitrateChange func(bitrate int)

	latestStats Stats
	minBitrate  int
	maxBitrate  int
}

// Option configures a bandwidth estimator.
//
// Deprecated: See comment on SendSideBWE.
type Option func(*SendSideBWE) error

// SendSideBWEInitialBitrate sets the initial bitrate of new GCC interceptors.
//
// Deprecated: See comment on SendSideBWE.
func SendSideBWEInitialBitrate(rate int) Option {
	return func(e *SendSideBWE) error {
		e.latestBitrate.Store(int64(rate))

		return nil
	}
}

// SendSideBWEMaxBitrate sets the initial bitrate of new GCC interceptors.
//
// Deprecated: See comment on SendSideBWE.
func SendSideBWEMaxBitrate(rate int) Option {
	return func(e *SendSideBWE) error {
		e.maxBitrate = rate

		return nil
	}
}

// SendSideBWEMinBitrate sets the initial bitrate of new GCC interceptors.
//
// Deprecated: See comment on SendSideBWE.
func SendSideBWEMinBitrate(rate int) Option {
	return func(e *SendSideBWE) error {
		e.minBitrate = rate

		return nil
	}
}

// SendSideBWEPacer sets the pacing algorithm to use.
//
// Deprecated: See comment on SendSideBWE.
func SendSideBWEPacer(p Pacer) Option {
	return func(e *SendSideBWE) error {
		e.pacer = p

		return nil
	}
}

// WithLoggerFactory sets the logger factory for the bandwidth estimator.
//
// Deprecated: See comment on SendSideBWE.
func WithLoggerFactory(factory logging.LoggerFactory) Option {
	return func(e *SendSideBWE) error {
		e.loggerFactory = factory

		return nil
	}
}

// NewSendSideBWE creates a new sender side bandwidth estimator.
//
// Deprecated: See comment on SendSideBWE.
func NewSendSideBWE(opts ...Option) (*SendSideBWE, error) {
	send := &SendSideBWE{
		pacer:                 nil,
		onTargetBitrateChange: nil,
		latestStats:           Stats{},
		minBitrate:            minBitrate,
		maxBitrate:            maxBitrate,
		latestBitrate:         atomic.Int64{},
		bwe:                   nil,
		loggerFactory:         nil,
	}
	send.latestBitrate.Store(latestBitrate)
	for _, opt := range opts {
		if err := opt(send); err != nil {
			return nil, err
		}
	}
	var err error
	send.bwe, err = gcc.NewSendSideController(send.minBitrate, send.minBitrate, send.maxBitrate)
	if err != nil {
		return nil, err
	}
	if send.loggerFactory == nil {
		send.loggerFactory = logging.NewDefaultLoggerFactory()
	}
	if send.pacer == nil {
		send.pacer = newLeakyBucketPacer(int(send.latestBitrate.Load()), send.loggerFactory)
	}

	return send, nil
}

// AddStream adds a new stream to the bandwidth estimator.
//
// Deprecated: See comment on SendSideBWE.
func (e *SendSideBWE) AddStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	var hdrExtID uint8
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			hdrExtID = uint8(e.ID) //nolint:gosec // G115

			break
		}
	}

	e.pacer.AddStream(info.SSRC, interceptor.RTPWriterFunc(
		func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
			if hdrExtID != 0 {
				if attributes == nil {
					attributes = make(interceptor.Attributes)
				}
				attributes.Set(cc.TwccExtensionAttributesKey, hdrExtID)
			}

			return writer.Write(header, payload, attributes)
		},
	))

	return e.pacer
}

// WriteRTCP adds some RTCP feedback to the bandwidth estimator.
//
// Deprecated: See comment on SendSideBWE.
func (e *SendSideBWE) WriteRTCP(pkts []rtcp.Packet, attr interceptor.Attributes) error {
	report, ok := attr.Get(rtpfb.CCFBAttributesKey).(rtpfb.Report)
	if ok {
		for _, pr := range report.PacketReports {
			if pr.Arrived {
				e.bwe.OnAck(
					pr.SequenceNumber,
					pr.Size,
					pr.Departure,
					pr.Arrival,
				)
			} else {
				e.bwe.OnLoss()
			}
		}
		rate := e.bwe.OnFeedback(report.Arrival, report.RTT)
		prev := e.latestBitrate.Swap(int64(rate))
		if e.pacer != nil {
			e.pacer.SetTargetBitrate(rate)
		}
		if rate != int(prev) && e.onTargetBitrateChange != nil {
			e.onTargetBitrateChange(rate)
		}
	}

	return nil
}

// GetTargetBitrate returns the current target bitrate in bits per second.
//
// Deprecated: See comment on SendSideBWE.
func (e *SendSideBWE) GetTargetBitrate() int {
	return int(e.latestBitrate.Load())
}

// GetStats returns some internal statistics of the bandwidth estimator.
//
// Deprecated: See comment on SendSideBWE.
func (e *SendSideBWE) GetStats() map[string]any {
	return map[string]any{
		"lossTargetBitrate":  0,
		"averageLoss":        0,
		"delayTargetBitrate": 0,
		"delayMeasurement":   0,
		"delayEstimate":      0,
		"delayThreshold":     0,
		"usage":              0,
		"state":              0,
	}
}

// OnTargetBitrateChange sets the callback that is called when the target
// bitrate in bits per second changes.
//
// Deprecated: See comment on SendSideBWE.
func (e *SendSideBWE) OnTargetBitrateChange(f func(bitrate int)) {
	e.onTargetBitrateChange = f
}

// Close stops and closes the bandwidth estimator.
//
// Deprecated: See comment on SendSideBWE.
func (e *SendSideBWE) Close() error {
	return e.pacer.Close()
}
