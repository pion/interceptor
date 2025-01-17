package bwe

import (
	"time"

	"github.com/pion/logging"
)

type SendSideController struct {
	log  logging.LeveledLogger
	dre  *deliveryRateEstimator
	lbc  *LossRateController
	drc  *DelayRateController
	rate int
}

func NewSendSideController(initialRate, minRate, maxRate int) *SendSideController {
	return &SendSideController{
		log:  logging.NewDefaultLoggerFactory().NewLogger("bwe_send_side_controller"),
		dre:  newDeliveryRateEstimator(time.Second),
		lbc:  NewLossRateController(initialRate, minRate, maxRate),
		drc:  NewDelayRateController(initialRate),
		rate: initialRate,
	}
}

func (c *SendSideController) OnAcks(arrival time.Time, rtt time.Duration, acks []Acknowledgment) int {
	if len(acks) == 0 {
		return c.rate
	}

	for _, ack := range acks {
		if ack.Arrived {
			c.lbc.OnPacketAcked()
			if !ack.Arrival.IsZero() {
				c.dre.OnPacketAcked(ack.Arrival, int(ack.Size))
				c.drc.OnPacketAcked(ack)
			}
		} else {
			c.lbc.OnPacketLost()
		}
	}

	delivered := c.dre.GetRate()
	lossTarget := c.lbc.Update(delivered)
	delayTarget := c.drc.Update(arrival, delivered, rtt)
	c.rate = min(lossTarget, delayTarget)
	c.log.Tracef("rtt=%v, delivered=%v, lossTarget=%v, delayTarget=%v, target=%v", rtt.Nanoseconds(), delivered, lossTarget, delayTarget, c.rate)
	return c.rate
}
