package gcc

import (
	"math"
	"time"
)

const (
	beta                = 0.85
	overuseTimeTh       = 10 * time.Millisecond
	overuseCoefficientU = 0.0018
	overuseCoefficientD = 0.01
	initialDelayVarTh   = 6
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

	history   []Acknowledgment
	lastGroup *arrivalGroup
	state     int
	delVarTh  float64

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

func newDelayBasedBWE(initialBitrate int) *delayBasedBandwidthEstimator {
	e := &delayBasedBandwidthEstimator{
		estimator:         newKalman(),
		lastBitrateUpdate: time.Time{},
		bitrate:           initialBitrate,
		history:           []Acknowledgment{},
		lastGroup:         nil,
		state:             increase,
		delVarTh:          initialDelayVarTh,
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

func (e *delayBasedBandwidthEstimator) incomingFeedbackInternal(p []Acknowledgment) {
	// TODO: Improve mechanism to delete old entries.
	// How many to keep?
	if len(p) == 0 {
		return
	}
	latestAckArrival := p[len(p)-1].Arrival
	for _, h := range e.history {
		if latestAckArrival.Sub(h.Arrival) > 500*time.Millisecond {
			e.history = e.history[1:]
		}
	}
	for _, ack := range p {
		if !ack.Arrival.IsZero() {
			e.history = append(e.history, ack)
		}
	}
	e.estimateAll(preFilter(e.history))
}

func (e *delayBasedBandwidthEstimator) estimateAll(groups []arrivalGroup) {
	//	for i, g := range groups {
	//		ns := []uint16{}
	//		for _, pkt := range g.packets {
	//			ns = append(ns, pkt.Header.SequenceNumber)
	//		}
	//		fmt.Printf("group %v: [%v]\n", i, ns)
	//	}
	if len(groups) == 0 {
		return
	}
	if e.lastGroup == nil {
		e.lastGroup = &groups[0]
		return
	}

	d0 := interGroupDelayVariation(*e.lastGroup, groups[0])
	estimate := e.updateEstimate(float64(d0.Milliseconds()))
	e.updateState(e.detectOverUse(estimate, float64(groups[0].arrival.Sub(e.lastGroup.arrival).Milliseconds())))
	e.rtt = groups[0].rtt

	for i := 1; i < len(groups); i++ {
		dx := interGroupDelayVariation(groups[i-1], groups[i])
		estimate := e.updateEstimate(float64(dx.Milliseconds()))
		//fmt.Printf("dx=%v, estimate=%v\n", dx, estimate)
		e.updateState(e.detectOverUse(estimate, float64(groups[i].arrival.Sub(groups[i-1].arrival).Milliseconds())))
		e.rtt = groups[i].rtt
	}
	e.lastGroup = &groups[len(groups)-1]
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
	if e.state == hold {
		return DelayStats{
			State:     e.state,
			Bitrate:   e.bitrate,
			Estimate:  e.lastEstimate,
			Threshold: e.delVarTh,
			RTT:       e.rtt,
		}
	}

	if e.state == increase {
		e.increaseBitrate()
		e.lastBitrateUpdate = time.Now()
		return DelayStats{
			State:     e.state,
			Bitrate:   e.bitrate,
			Estimate:  e.lastEstimate,
			Threshold: e.delVarTh,
			RTT:       e.rtt,
		}
	}

	e.decreaseBitrate()
	e.lastBitrateUpdate = time.Now()
	return DelayStats{
		State:     e.state,
		Bitrate:   e.bitrate,
		Estimate:  e.lastEstimate,
		Threshold: e.delVarTh,
		RTT:       e.rtt,
	}
}

func (e *delayBasedBandwidthEstimator) decreaseBitrate() {
	r := calculateReceivedRate(e.history)

	e.bitrate = int(beta * float64(r))

	if e.decreaseEMA == 0 {
		e.decreaseEMA = float64(r)
	} else {
		d := float64(r) - e.decreaseEMA
		e.decreaseEMA = e.decreaseEMA + e.decreaseEMAAlpha*d
		e.decreaseEMAVar = (1 - e.decreaseEMAAlpha) * (e.decreaseEMAVar + e.decreaseEMAAlpha*d*d)
		e.decreaseStdDev = math.Sqrt(e.decreaseEMAVar)
	}
}

func (e *delayBasedBandwidthEstimator) increaseBitrate() {
	r := calculateReceivedRate(e.history)

	if float64(r) > e.decreaseEMA-3*e.decreaseStdDev &&
		float64(r) < e.decreaseEMA+3*e.decreaseStdDev {

		bitsPerFrame := float64(e.bitrate) / 30.0
		packetsPerFrame := math.Ceil(bitsPerFrame / (1200 * 8))
		expectedPacketSizeBits := bitsPerFrame / packetsPerFrame

		responseTimeInMs := 100 + 300.0
		alpha := 0.5 * math.Min(float64(time.Since(e.lastBitrateUpdate).Milliseconds())/responseTimeInMs, 1.0)
		increase := int(math.Max(1000.0, alpha*expectedPacketSizeBits))
		//fmt.Printf("additive increase br += %v\n", increase)
		e.bitrate += increase
		return
	}
	eta := math.Pow(1.08, math.Min(float64(time.Since(e.lastBitrateUpdate).Milliseconds())/1000, 1.0))
	//fmt.Printf("multiplicative increase br *= %v\n", eta)
	e.bitrate = int(eta * float64(e.bitrate))
}

func (e *delayBasedBandwidthEstimator) detectOverUse(estimate, dt float64) int {
	k := overuseCoefficientU
	absEstimate := math.Abs(estimate)
	if absEstimate < e.delVarTh {
		k = overuseCoefficientD
	}
	if absEstimate-e.delVarTh <= 15 {
		e.delVarTh = e.delVarTh + dt*k*(absEstimate-e.delVarTh)
	}
	e.delVarTh = math.Min(e.delVarTh, 60)
	e.delVarTh = math.Max(e.delVarTh, 1)

	defer func() {
		e.lastEstimate = estimate
	}()

	if estimate > e.delVarTh && estimate >= e.lastEstimate {
		return overUse
	}

	if estimate < -e.delVarTh {
		return underUse
	}
	return normal
}

// calculateReceivedRate calculates the rate of RTP bytes in log between a and b
func calculateReceivedRate(log []Acknowledgment) int {
	if len(log) == 0 {
		return 0
	}
	sum := 0
	start := log[0].Arrival
	end := log[len(log)-1].Arrival

	for _, ack := range log {
		if !ack.Arrival.IsZero() {
			sum += ack.Header.MarshalSize() + ack.Size
		}
	}

	d := end.Sub(start)
	rate := int(float64(8*sum) / d.Seconds())
	//fmt.Printf("calculating rate for: from %v to %v => %v / %v = %v\n", start, end, sum, d.Seconds(), rate)
	return rate
}

func preFilter(log []Acknowledgment) []arrivalGroup {
	res := []arrivalGroup{}
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

		if interDepartureTimePkt(res[len(res)-1], p) < 5*time.Millisecond {
			res[len(res)-1].add(p)
			continue
		}

		if interArrivalTimePkt(res[len(res)-1], p) < 5*time.Millisecond &&
			interGroupDelayVariationPkt(res[len(res)-1], p) < 0 {

			res[len(res)-1].add(p)
			continue
		}

		ag := arrivalGroup{}
		ag.add(p)
		res = append(res, ag)
	}
	//	for _, r := range res {
	//		for _, n := range r.packets {
	//			fmt.Printf("%v:%v\n", n.TLCC, n.Arrival.Sub(time.Time{}).Milliseconds())
	//		}
	//	}
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
	//fmt.Printf("b.arrival - a.arrival: %v - %v = %v\n", b.arrival.UnixMilli(), a.arrival.UnixMilli(), b.arrival.Sub(a.arrival))
	//fmt.Printf("b.departure - a.departure: %v - %v = %v\n", b.departure.UnixMilli(), a.departure.UnixMilli(), b.departure.Sub(a.departure))
	return b.arrival.Sub(a.arrival) - b.departure.Sub(a.departure)
}
