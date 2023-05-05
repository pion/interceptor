// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"sync"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/logging"
)

// DelayStats contains some internal statistics of the delay based congestion
// controller
type DelayStats struct {
	Measurement      time.Duration
	Estimate         time.Duration
	Threshold        time.Duration
	LastReceiveDelta time.Duration

	Usage         usage
	State         state
	TargetBitrate int
}

type now func() time.Time

type delayController struct {
	ackPipe     chan<- []cc.Acknowledgment
	ackRatePipe chan<- []cc.Acknowledgment

	*arrivalGroupAccumulator
	*rateController

	onUpdateCallback func(DelayStats)

	wg sync.WaitGroup

	log logging.LeveledLogger
}

type delayControllerConfig struct {
	nowFn          now
	initialBitrate int
	minBitrate     int
	maxBitrate     int
}

func newDelayController(c delayControllerConfig) *delayController {
	ackPipe := make(chan []cc.Acknowledgment)
	ackRatePipe := make(chan []cc.Acknowledgment)

	delayController := &delayController{
		ackPipe:                 ackPipe,
		ackRatePipe:             ackRatePipe,
		arrivalGroupAccumulator: nil,
		rateController:          nil,
		onUpdateCallback:        nil,
		wg:                      sync.WaitGroup{},
		log:                     logging.NewDefaultLoggerFactory().NewLogger("gcc_delay_controller"),
	}

	rateController := newRateController(c.nowFn, c.initialBitrate, c.minBitrate, c.maxBitrate, func(ds DelayStats) {
		delayController.log.Infof("delaystats: %v", ds)
		if delayController.onUpdateCallback != nil {
			delayController.onUpdateCallback(ds)
		}
	})
	delayController.rateController = rateController
	overuseDetector := newOveruseDetector(newAdaptiveThreshold(), 10*time.Millisecond, rateController.onDelayStats)
	slopeEstimator := newSlopeEstimator(newKalman(), overuseDetector.onDelayStats)
	arrivalGroupAccumulator := newArrivalGroupAccumulator()

	rc := newRateCalculator(500 * time.Millisecond)

	delayController.wg.Add(2)
	go func() {
		defer delayController.wg.Done()
		arrivalGroupAccumulator.run(ackPipe, slopeEstimator.onArrivalGroup)
	}()
	go func() {
		defer delayController.wg.Done()
		rc.run(ackRatePipe, rateController.onReceivedRate)
	}()

	return delayController
}

func (d *delayController) onUpdate(f func(DelayStats)) {
	d.onUpdateCallback = f
}

func (d *delayController) updateDelayEstimate(acks []cc.Acknowledgment) {
	d.ackPipe <- acks
	d.ackRatePipe <- acks
}

func (d *delayController) Close() error {
	defer d.wg.Wait()

	close(d.ackPipe)
	close(d.ackRatePipe)

	return nil
}
