// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"math"
	"time"

	"github.com/pion/rtp"
)

const (
	defaultDynamicMinPackets   = 1
	defaultDynamicStepPackets  = 5
	defaultUnderflowsPerMinute = 3
	defaultTargetDelay         = 100 * time.Millisecond
	underflowWindow            = time.Minute
	packetSizeEWMAWeight       = 16
)

// BandwidthEstimator provides the estimated bitrate used to size a dynamic
// JitterBuffer. cc.BandwidthEstimator satisfies it.
type BandwidthEstimator interface {
	// GetTargetBitrate returns the bitrate in bits per second; <= 0 means unknown.
	GetTargetBitrate() int
}

// DynamicLengthConfig configures WithDynamicLength. Zero fields use defaults.
type DynamicLengthConfig struct {
	// MinPackets is the smallest length. Default 1.
	MinPackets uint16
	// MaxPackets is the largest length. Default and cap: the buffer size (see WithSize).
	MaxPackets uint16
	// StepPackets is how much the length grows per adjustment. Default 5.
	StepPackets uint16
	// UnderflowsPerMinute is the number of underflows within a trailing minute
	// that grows the length. Default 3.
	UnderflowsPerMinute uint16
	// TargetDelay is the playout delay covered when sizing from the estimator.
	// Default 100ms.
	TargetDelay time.Duration
}

// WithDynamicLength makes the buffer length adapt while buffering:
//
//   - With a BandwidthEstimator reporting a positive bitrate, the base length is
//     ceil(bitrate * TargetDelay / (8 * average packet MarshalSize())). Otherwise
//     it is the static count from WithMinimumPacketCount.
//   - Each time UnderflowsPerMinute underflows occur within a trailing minute,
//     StepPackets is added to the length permanently. The total is clamped to
//     [MinPackets, MaxPackets].
//   - An underflow (a pop from an empty buffer) returns the buffer to Buffering,
//     so playout resumes once it has refilled to the current length.
//   - A lost packet is skipped at playout time instead of stalling Pop.
//
// Clear(true) resets the growth and packet size average; the estimator is kept.
func WithDynamicLength(cfg DynamicLengthConfig) Option {
	return func(jb *JitterBuffer) {
		jb.dynamicCfg = &cfg
	}
}

// WithBandwidthEstimator sets the estimator that sizes a dynamic buffer (see
// WithDynamicLength), enabling dynamic mode with the default configuration if
// WithDynamicLength is not given. GetTargetBitrate is read in bits per second,
// and packets are sized by RTP MarshalSize(): header, CSRCs, extensions and
// payload. When passed through the interceptor's WithBufferOptions, the one
// estimator is shared by every remote stream.
func WithBandwidthEstimator(estimator BandwidthEstimator) Option {
	return func(jb *JitterBuffer) {
		jb.estimator = estimator
	}
}

type dynamicLength struct {
	cfg           DynamicLengthConfig
	estimator     BandwidthEstimator
	baseCount     uint16
	extra         uint16
	underflows    []time.Time
	avgPacketSize float64
	now           func() time.Time
}

func newDynamicLength(
	cfg DynamicLengthConfig, estimator BandwidthEstimator, baseCount, bufferSize uint16,
) *dynamicLength {
	if cfg.MinPackets == 0 {
		cfg.MinPackets = defaultDynamicMinPackets
	}
	if cfg.MaxPackets == 0 || cfg.MaxPackets > bufferSize {
		cfg.MaxPackets = bufferSize
	}
	cfg.MinPackets = min(cfg.MinPackets, cfg.MaxPackets)
	if cfg.StepPackets == 0 {
		cfg.StepPackets = defaultDynamicStepPackets
	}
	if cfg.UnderflowsPerMinute == 0 {
		cfg.UnderflowsPerMinute = defaultUnderflowsPerMinute
	}
	if cfg.TargetDelay <= 0 {
		cfg.TargetDelay = defaultTargetDelay
	}

	return &dynamicLength{cfg: cfg, estimator: estimator, baseCount: baseCount, now: time.Now}
}

func (d *dynamicLength) observe(packet *rtp.Packet) {
	size := float64(packet.MarshalSize())
	if d.avgPacketSize == 0 {
		d.avgPacketSize = size
	} else {
		d.avgPacketSize += (size - d.avgPacketSize) / packetSizeEWMAWeight
	}
}

func (d *dynamicLength) target() uint16 {
	base := min(d.baseCount, d.cfg.MaxPackets)
	if d.estimator != nil && d.avgPacketSize > 0 {
		if bitrate := d.estimator.GetTargetBitrate(); bitrate > 0 {
			packets := math.Ceil(float64(bitrate) * d.cfg.TargetDelay.Seconds() / (8 * d.avgPacketSize))
			base = uint16(min(packets, float64(d.cfg.MaxPackets)))
		}
	}
	total := base + min(d.extra, d.cfg.MaxPackets-base)

	return max(total, d.cfg.MinPackets)
}

func (d *dynamicLength) onUnderflow() {
	now := d.now()
	expired := 0
	for expired < len(d.underflows) && !now.Before(d.underflows[expired].Add(underflowWindow)) {
		expired++
	}
	d.underflows = append(d.underflows[expired:], now)
	if len(d.underflows) < int(d.cfg.UnderflowsPerMinute) {
		return
	}
	d.extra += min(d.cfg.StepPackets, d.cfg.MaxPackets-d.extra)
	d.underflows = d.underflows[:0]
}

func (d *dynamicLength) reset() {
	d.extra = 0
	d.avgPacketSize = 0
	d.underflows = d.underflows[:0]
}
