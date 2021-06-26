package gcc

import (
	"sync"
	"time"

	"github.com/pion/interceptor/internal/cc"
)

const (
	decreaseEMAAlpha = 0.95
	beta             = 0.85
)

// DelayStats contains some internal statistics of the delay based congestion
// controller
type DelayStats struct {
	Measurement      time.Duration
	Estimate         time.Duration
	Threshold        time.Duration
	lastReceiveDelta time.Duration

	Usage         usage
	State         state
	TargetBitrate int
	RTT           time.Duration
}

type estimator interface {
	updateEstimate(measurement time.Duration) time.Duration
}

type estimatorFunc func(time.Duration) time.Duration

func (f estimatorFunc) updateEstimate(d time.Duration) time.Duration {
	return f(d)
}

type now func() time.Time

type delayController struct {
	ackPipe     chan<- cc.Acknowledgment
	ackRatePipe chan<- cc.Acknowledgment
	ackRTTPipe  chan<- []cc.Acknowledgment

	onUpdateCallback func(DelayStats)

	wg sync.WaitGroup
}

type delayControllerConfig struct {
	nowFn          now
	initialBitrate int
	minBitrate     int
	maxBitrate     int
}

func newDelayController(c delayControllerConfig) *delayController {
	ackPipe := make(chan cc.Acknowledgment)
	ackRatePipe := make(chan cc.Acknowledgment)
	ackRTTPipe := make(chan []cc.Acknowledgment)

	delayController := &delayController{
		ackPipe:     ackPipe,
		ackRatePipe: ackRatePipe,
		ackRTTPipe:  ackRTTPipe,
		wg:          sync.WaitGroup{},
	}

	rc := newRateCalculator(500 * time.Millisecond)
	re := newRTTEstimator()

	reaceivedRate := rc.run(ackRatePipe)
	rtt := re.run(ackRTTPipe)

	arrivalGroupAccumulator := newArrivalGroupAccumulator()
	slopeEstimator := newSlopeEstimator(newKalman())
	overuseDetector := newOveruseDetector(newAdaptiveThreshold(), 10*time.Millisecond)
	rateController := newRateController(c.nowFn, c.initialBitrate, c.minBitrate, c.maxBitrate)

	arrival := arrivalGroupAccumulator.run(ackPipe)
	estimate := slopeEstimator.run(arrival)
	state := overuseDetector.run(estimate)
	delayStats := rateController.run(state, reaceivedRate, rtt)
	delayController.loop(delayStats)

	return delayController
}

func (d *delayController) onUpdate(f func(DelayStats)) {
	d.onUpdateCallback = f
}

func (d *delayController) updateDelayEstimate(acks []cc.Acknowledgment) {
	for _, ack := range acks {
		d.ackPipe <- ack
		d.ackRatePipe <- ack
	}
	d.ackRTTPipe <- acks
}

func (d *delayController) loop(in chan DelayStats) {
	d.wg.Add(1)
	defer d.wg.Done()

	go func() {
		for next := range in {
			if d.onUpdateCallback != nil {
				d.onUpdateCallback(next)
			}
		}
	}()
}

func (d *delayController) Close() error {
	close(d.ackPipe)
	close(d.ackRTTPipe)
	close(d.ackRatePipe)
	d.wg.Wait()
	return nil
}

func interArrivalTimePkt(a arrivalGroup, b cc.Acknowledgment) time.Duration {
	return b.Arrival.Sub(a.arrival)
}

func interDepartureTimePkt(a arrivalGroup, b cc.Acknowledgment) time.Duration {
	if len(a.packets) == 0 {
		return 0
	}
	return b.Departure.Sub(a.packets[len(a.packets)-1].Departure)
}

func interGroupDelayVariationPkt(a arrivalGroup, b cc.Acknowledgment) time.Duration {
	return b.Arrival.Sub(a.arrival) - b.Departure.Sub(a.departure)
}

func interGroupDelayVariation(a, b arrivalGroup) time.Duration {
	return b.arrival.Sub(a.arrival) - b.departure.Sub(a.departure)
}
