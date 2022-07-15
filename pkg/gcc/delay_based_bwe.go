package gcc

import (
	"sync"
	"time"

	"github.com/pion/interceptor/internal/cc"
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

type now func() time.Time

type delayController struct {
	ackPipe     chan<- cc.Acknowledgment
	ackRatePipe chan<- cc.Acknowledgment
	ackRTTPipe  chan<- []cc.Acknowledgment

	*arrivalGroupAccumulator

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
		ackPipe:                 ackPipe,
		ackRatePipe:             ackRatePipe,
		ackRTTPipe:              ackRTTPipe,
		arrivalGroupAccumulator: nil,
		onUpdateCallback:        nil,
		wg:                      sync.WaitGroup{},
	}

	rateController := newRateController(c.nowFn, c.initialBitrate, c.minBitrate, c.maxBitrate, func(ds DelayStats) {
		if delayController.onUpdateCallback != nil {
			delayController.onUpdateCallback(ds)
		}
	})
	overuseDetector := newOveruseDetector(newAdaptiveThreshold(), 10*time.Millisecond, rateController.onDelayStats)
	slopeEstimator := newSlopeEstimator(newKalman(), overuseDetector.onDelayStats)
	arrivalGroupAccumulator := newArrivalGroupAccumulator()

	rc := newRateCalculator(500 * time.Millisecond)
	re := newRTTEstimator()

	delayController.wg.Add(3)
	go func() {
		defer delayController.wg.Done()
		arrivalGroupAccumulator.run(ackPipe, slopeEstimator.onArrivalGroup)
	}()
	go func() {
		defer delayController.wg.Done()
		rc.run(ackRatePipe, rateController.onReceivedRate)
	}()
	go func() {
		defer delayController.wg.Done()
		re.run(ackRTTPipe, rateController.onRTT)
	}()

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

func (d *delayController) Close() error {
	defer d.wg.Wait()

	close(d.ackPipe)
	close(d.ackRTTPipe)
	close(d.ackRatePipe)

	return nil
}
