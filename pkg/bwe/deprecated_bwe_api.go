package bwe

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/ccfb"
	"github.com/pion/rtcp"
)

// GCCFactory creates a new cc.BandwidthEstimator
func GCCFactory() (cc.BandwidthEstimator, error) {
	return &GCC{
		lock: sync.Mutex{},
		sbwe: NewSendSideController(1_000_000, 100_000, 100_000_000),
		rate: 1_000_000,
	}, nil
}

// GCC implements cc.BandwidthEstimator
type GCC struct {
	lock     sync.Mutex
	sbwe     *SendSideController
	rate     int
	updateCB func(int)
}

// AddStream implements cc.BandwidthEstimator.
// Called by cc.Interceptor
func (g *GCC) AddStream(_ *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return writer
}

// Close implements cc.BandwidthEstimator.
// Called by cc.Interceptor
func (g *GCC) Close() error {
	// GCC does not need to be closed
	return nil
}

// GetStats implements cc.BandwidthEstimator.
// Called by application
func (g *GCC) GetStats() map[string]interface{} {
	g.lock.Lock()
	defer g.lock.Unlock()
	return map[string]interface{}{
		"warning":            "GetStats is deprecated",
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

// GetTargetBitrate implements cc.BandwidthEstimator.
// Called by application
func (g *GCC) GetTargetBitrate() int {
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.rate
}

// OnTargetBitrateChange implements cc.BandwidthEstimator.
// Called by application
func (g *GCC) OnTargetBitrateChange(f func(bitrate int)) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.updateCB = f
}

// WriteRTCP implements cc.BandwidthEstimator.
// Called by cc.Interceptor
func (g *GCC) WriteRTCP(_ []rtcp.Packet, attr interceptor.Attributes) error {
	reports, ok := attr.Get(ccfb.CCFBAttributesKey).([]ccfb.Report)
	if !ok {
		return errors.New("warning: GCC requires CCFB interceptor to be configured before the CC interceptor")
	}
	now := time.Now()
	for _, report := range reports {
		acks, rtt := readReport(report)
		g.update(now, rtt, acks)
	}
	return nil
}

func (g *GCC) update(now time.Time, rtt time.Duration, acks []Acknowledgment) {
	g.lock.Lock()
	defer g.lock.Unlock()
	oldRate := g.rate

	g.rate = g.sbwe.OnAcks(now, rtt, acks)

	if oldRate != g.rate && g.updateCB != nil {
		g.updateCB(g.rate)
	}
}

func readReport(report ccfb.Report) ([]Acknowledgment, time.Duration) {
	acks := []Acknowledgment{}
	latestAcked := Acknowledgment{}
	for _, prs := range report.SSRCToPacketReports {
		for _, pr := range prs {
			ack := Acknowledgment{
				SeqNr:     pr.SeqNr,
				Size:      pr.Size,
				Departure: pr.Departure,
				Arrived:   pr.Arrived,
				Arrival:   pr.Arrival,
				ECN:       ECN(pr.ECN),
			}
			if ack.Arrival.After(latestAcked.Arrival) {
				latestAcked = ack
			}
			acks = append(acks, ack)
		}
	}
	rtt := MeasureRTT(report.Departure, report.Arrival, latestAcked.Departure, latestAcked.Arrival)
	return acks, rtt
}
