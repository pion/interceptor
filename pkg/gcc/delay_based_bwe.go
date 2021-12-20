package gcc

import (
	"math"
	"time"
)

const (
	beta                = 0.85
	overuseCoefficientU = 0.01
	overuseCoefficientD = 0.0018
	initialDelayVarTh   = 1.25
	delVarThMin         = 0.6
	delVarThMax         = 60
)

const (
	increase = iota
	decrease
	hold
)

const (
	overUse = iota
	underUse
	normal
)

// DelayStats contains some internal statistics of the delay based congestion
// controller
type DelayStats struct {
	State     int
	Bitrate   int
	Estimate  float64
	Threshold float64
	RTT       time.Duration
}

type arrivalGroup struct {
	packets   []Acknowledgment
	arrival   time.Time
	departure time.Time
	rtt       time.Duration
}

func (g *arrivalGroup) add(a Acknowledgment) {
	g.packets = append(g.packets, a)
	g.arrival = a.Arrival
	g.departure = a.Departure
	g.rtt = a.RTT
}

type estimator interface {
	updateEstimate(measurement float64) float64
}

type delayBasedBandwidthEstimator struct {
	estimator

	lastBitrateUpdate time.Time
	bitrate           int

	receivedRate rateCalculator

	lastGroup      *arrivalGroup
	state          int
	delVarTh       float64
	inOverUse      bool
	inOverUseSince time.Time

	lastEstimate float64

	decreaseEMAAlpha float64
	decreaseEMA      float64
	decreaseEMAVar   float64
	decreaseStdDev   float64

	rtt time.Duration

	delayStats chan DelayStats
	feedback   chan []Acknowledgment
	close      chan struct{}
}

type rateCalculator struct {
	history []Acknowledgment
	window  time.Duration
	rate    int
}

func (rc *rateCalculator) update(acks []Acknowledgment) {
	rc.history = append(rc.history, acks...)
	sum := 0
	del := 0
	if len(rc.history) == 0 {
		rc.rate = 0
		return
	}
	now := rc.history[len(rc.history)-1].Arrival
	for _, ack := range rc.history {
		if now.Sub(ack.Arrival) > rc.window {
			del++
			continue
		}
		if !ack.Arrival.IsZero() {
			sum += ack.Size
		}
	}
	rc.history = rc.history[del:]
	rc.rate = int(float64(8*sum) / rc.window.Seconds())
}

func newDelayBasedBWE(initialBitrate int) *delayBasedBandwidthEstimator {
	e := &delayBasedBandwidthEstimator{
		estimator:         newKalman(),
		lastBitrateUpdate: time.Time{},
		bitrate:           initialBitrate,
		receivedRate:      rateCalculator{history: []Acknowledgment{}, window: 500 * time.Millisecond, rate: 0},
		lastGroup:         nil,
		state:             increase,
		delVarTh:          initialDelayVarTh,
		inOverUse:         false,
		inOverUseSince:    time.Time{},
		lastEstimate:      0,
		decreaseEMAAlpha:  0.95,
		decreaseEMA:       0,
		decreaseEMAVar:    0,
		decreaseStdDev:    0,
		rtt:               0,
		delayStats:        make(chan DelayStats),
		feedback:          make(chan []Acknowledgment),
		close:             make(chan struct{}),
	}
	go e.loop()
	return e
}

func (e *delayBasedBandwidthEstimator) Close() error {
	close(e.close)
	return nil
}

func (e *delayBasedBandwidthEstimator) getEstimate() DelayStats {
	return <-e.delayStats
}

func (e *delayBasedBandwidthEstimator) incomingFeedback(p []Acknowledgment) {
	e.feedback <- p
}

func (e *delayBasedBandwidthEstimator) loop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	delayStats := DelayStats{}
	for {
		select {
		case <-e.close:
			return
		case e.delayStats <- delayStats:
		case <-ticker.C:
			delayStats = e.getEstimateInternal()
		case fb := <-e.feedback:
			e.incomingFeedbackInternal(fb)
			delayStats = e.getEstimateInternal()
		}
	}
}

func getTLCC(g []Acknowledgment) []uint16 {
	t := []uint16{}
	for _, p := range g {
		t = append(t, p.TLCC)
	}
	return t
}

func (e *delayBasedBandwidthEstimator) incomingFeedbackInternal(p []Acknowledgment) {
	e.receivedRate.update(p)
	groups := preFilter(e.lastGroup, p)
	if len(groups) == 0 {
		return
	}
	if e.lastGroup != nil {
		e.estimateAll(groups[:len(groups)-1])
		e.lastGroup = &groups[len(groups)-1]
		return
	}
	e.estimateAll(groups[:len(groups)-1])
	e.lastGroup = &groups[len(groups)-1]
}

func (e *delayBasedBandwidthEstimator) estimateAll(groups []arrivalGroup) {
	for i := 1; i < len(groups)-1; i++ {
		dx := float64(interGroupDelayVariation(groups[i-1], groups[i]).Microseconds()) / 1000.0
		estimate := e.updateEstimate(dx)
		dt := float64(groups[i].arrival.Sub(groups[i-1].arrival).Milliseconds())
		e.updateState(e.detectOverUse(estimate, dt))
		e.rtt = groups[i].rtt
	}
}

func (e *delayBasedBandwidthEstimator) updateState(use int) {
	switch e.state {
	case hold:
		switch use {
		case overUse:
			e.state = decrease
			return
		case normal:
			e.state = increase
			return
		case underUse:
			return
		}

	case increase:
		switch use {
		case overUse:
			e.state = decrease
			return
		case normal:
			return
		case underUse:
			e.state = hold
			return
		}

	case decrease:
		switch use {
		case overUse:
			return
		case normal:
			e.state = hold
			return
		case underUse:
			e.state = hold
		default:
			return
		}
	}
}

func (e *delayBasedBandwidthEstimator) getEstimateInternal() DelayStats {
	switch e.state {
	case hold:
	case increase:
		e.increaseBitrate()
		e.lastBitrateUpdate = time.Now()
	case decrease:
		e.decreaseBitrate()
		e.lastBitrateUpdate = time.Now()
	}

	return DelayStats{
		State:     e.state,
		Bitrate:   e.bitrate,
		Estimate:  e.lastEstimate,
		Threshold: e.delVarTh,
		RTT:       e.rtt,
	}
}

func (e *delayBasedBandwidthEstimator) decreaseBitrate() {
	r := e.receivedRate.rate

	e.bitrate = int(beta * float64(r))

	if e.decreaseEMA == 0 {
		e.decreaseEMA = float64(r)
	} else {
		d := float64(r) - e.decreaseEMA
		e.decreaseEMA += e.decreaseEMAAlpha * d
		e.decreaseEMAVar = (1 - e.decreaseEMAAlpha) * (e.decreaseEMAVar + e.decreaseEMAAlpha*d*d)
		e.decreaseStdDev = math.Sqrt(e.decreaseEMAVar)
	}
}

func (e *delayBasedBandwidthEstimator) increaseBitrate() {
	r := e.receivedRate.rate

	if float64(r) > e.decreaseEMA-3*e.decreaseStdDev &&
		float64(r) < e.decreaseEMA+3*e.decreaseStdDev {
		bitsPerFrame := float64(e.bitrate) / 30.0
		packetsPerFrame := math.Ceil(bitsPerFrame / (1200 * 8))
		expectedPacketSizeBits := bitsPerFrame / packetsPerFrame

		responseTimeInMs := 100 + 300.0
		alpha := 0.5 * math.Min(float64(time.Since(e.lastBitrateUpdate).Milliseconds())/responseTimeInMs, 1.0)
		increase := int(math.Max(1000.0, alpha*expectedPacketSizeBits))
		e.bitrate += increase
		return
	}
	eta := math.Pow(1.08, math.Min(float64(time.Since(e.lastBitrateUpdate).Milliseconds())/1000, 1.0))
	e.bitrate = int(eta * float64(e.bitrate))
}

func (e *delayBasedBandwidthEstimator) detectOverUse(estimate, dt float64) int {
	use := normal
	if estimate > e.delVarTh && estimate >= e.lastEstimate {
		if time.Since(e.inOverUseSince) > 10*time.Millisecond {
			use = overUse
		}
		e.inOverUse = true
	} else if estimate < -e.delVarTh {
		use = underUse
		e.inOverUse = false
	} else {
		e.inOverUse = false
	}

	k := overuseCoefficientU
	absEstimate := math.Abs(estimate)
	if absEstimate < e.delVarTh {
		k = overuseCoefficientD
	}
	if absEstimate-e.delVarTh <= 15 {
		inc := dt * k * (absEstimate - e.delVarTh)
		e.delVarTh += inc
	}
	e.delVarTh = math.Min(e.delVarTh, delVarThMax)
	e.delVarTh = math.Max(e.delVarTh, delVarThMin)

	e.lastEstimate = estimate

	return use
}

func preFilter(lastKnown *arrivalGroup, log []Acknowledgment) []arrivalGroup {
	res := []arrivalGroup{}
	if lastKnown != nil {
		res = append(res, *lastKnown)
	}
	for _, p := range log {
		if p.Arrival.IsZero() {
			continue
		}
		if len(res) == 0 {
			ag := arrivalGroup{}
			ag.add(p)
			res = append(res, ag)
			continue
		}

		if interDepartureTimePkt(res[len(res)-1], p) <= 5*time.Millisecond {
			res[len(res)-1].add(p)
			continue
		}

		if interArrivalTimePkt(res[len(res)-1], p) <= 5*time.Millisecond &&
			interGroupDelayVariationPkt(res[len(res)-1], p) < 0 {
			res[len(res)-1].add(p)
			continue
		}

		ag := arrivalGroup{}
		ag.add(p)
		res = append(res, ag)
	}
	return res
}

func interArrivalTimePkt(a arrivalGroup, b Acknowledgment) time.Duration {
	return b.Arrival.Sub(a.arrival)
}

func interDepartureTimePkt(a arrivalGroup, b Acknowledgment) time.Duration {
	if len(a.packets) == 0 {
		return 0
	}
	return b.Departure.Sub(a.packets[0].Departure)
}

func interGroupDelayVariationPkt(a arrivalGroup, b Acknowledgment) time.Duration {
	return b.Arrival.Sub(a.arrival) - b.Departure.Sub(a.departure)
}

func interGroupDelayVariation(a, b arrivalGroup) time.Duration {
	return b.arrival.Sub(a.arrival) - b.departure.Sub(a.departure)
}
