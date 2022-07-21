package gcc

import (
	"time"

	"github.com/pion/interceptor/internal/cc"
)

type rateCalculator struct {
	// config
	maxWindowPackets        int
	minWindowDuration       time.Duration
	maxWindowDuration       time.Duration
	sendRateRequiredPackets int

	// state
	history                  []cc.Acknowledgment
	largestDiscardedSendTime time.Time
}

func newRateCalculator() *rateCalculator {
	return &rateCalculator{
		maxWindowPackets:         500,
		minWindowDuration:        time.Second,
		maxWindowDuration:        5 * time.Second,
		sendRateRequiredPackets:  10,
		history:                  []cc.Acknowledgment{},
		largestDiscardedSendTime: time.Time{},
	}
}

func (c *rateCalculator) run(in <-chan []cc.Acknowledgment, onRateUpdate func(int)) {
	for acks := range in {
		for _, ack := range acks {
			if ack.Departure.IsZero() || ack.Arrival.IsZero() {
				continue
			}

			c.history = append(c.history, ack)
		}
		for c.firstPacketOutsideWindow() {
			if c.history[0].Departure.After(c.largestDiscardedSendTime) {
				c.largestDiscardedSendTime = c.history[0].Departure
			}
			c.history = c.history[1:]
		}

		if len(c.history) == 0 || len(c.history) < c.sendRateRequiredPackets {
			continue
		}

		largestReceivGap := time.Duration(0)
		secondLargestReceiveGap := time.Duration(0)
		for i := 1; i < len(c.history); i++ {
			gap := c.history[1].Arrival.Sub(c.history[i-1].Arrival)
			if gap > largestReceivGap {
				secondLargestReceiveGap = largestReceivGap
				largestReceivGap = gap
			} else if gap > secondLargestReceiveGap {
				secondLargestReceiveGap = gap
			}
		}

		firstSendTime := time.Time{}
		var lastSendTime time.Time
		var firstReceiveTime time.Time
		var lastReceiveTime time.Time
		receiveSize := 0
		sendSize := 0
		firstReceiveSize := 0
		lastSendSize := 0
		numSentPacketsInWindow := 0
		for _, ack := range c.history {
			if firstReceiveTime.IsZero() || ack.Arrival.Before(firstReceiveTime) {
				firstReceiveTime = ack.Arrival
				firstReceiveSize = ack.Size
			}
			if lastReceiveTime.IsZero() || ack.Arrival.After(lastReceiveTime) {
				lastReceiveTime = ack.Arrival
			}
			receiveSize += ack.Size

			if ack.Departure.Before(c.largestDiscardedSendTime) {
				continue
			}
			if lastSendTime.IsZero() || ack.Departure.After(lastSendTime) {
				lastSendTime = ack.Departure
				lastSendSize = ack.Size
			}
			if firstSendTime.IsZero() || ack.Departure.Before(firstSendTime) {
				firstSendTime = ack.Departure
			}
			sendSize += ack.Size
			numSentPacketsInWindow++
		}
		receiveSize -= firstReceiveSize
		sendSize -= lastSendSize

		receiveDuration := lastReceiveTime.Sub(firstReceiveTime) - (largestReceivGap + secondLargestReceiveGap)
		receiveRate := 8 * float64(receiveSize) / receiveDuration.Seconds()
		if numSentPacketsInWindow < c.sendRateRequiredPackets {
			onRateUpdate(int(receiveRate))
			continue
		}

		sendDuration := lastSendTime.Sub(firstSendTime)
		if sendDuration == 0 {
			sendDuration = time.Millisecond
		}
		sendRate := 8 * float64(sendSize) / sendDuration.Seconds()

		if receiveRate < sendRate {
			onRateUpdate(int(receiveRate))
			continue
		}
		onRateUpdate(int(sendRate))
	}
}

func (c *rateCalculator) firstPacketOutsideWindow() bool {
	if len(c.history) == 0 {
		return false
	}
	if len(c.history) > c.maxWindowPackets {
		return true
	}

	currentWindow := c.history[len(c.history)-1].Arrival.Sub(c.history[0].Arrival)
	if currentWindow > c.maxWindowDuration {
		return true
	}
	if len(c.history) > c.maxWindowPackets && currentWindow > c.minWindowDuration {
		return true
	}
	return false
}
