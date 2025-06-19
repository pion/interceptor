package bwe

type LossRateController struct {
	bitrate  int
	min, max float64

	packetsSinceLastUpdate int
	arrivedSinceLastUpdate int
	lostSinceLastUpdate    int
}

func NewLossRateController(initialRate, minRate, maxRate int) *LossRateController {
	return &LossRateController{
		bitrate:                initialRate,
		min:                    float64(minRate),
		max:                    float64(maxRate),
		packetsSinceLastUpdate: 0,
		arrivedSinceLastUpdate: 0,
		lostSinceLastUpdate:    0,
	}
}

func (l *LossRateController) OnPacketAcked() {
	l.packetsSinceLastUpdate++
	l.arrivedSinceLastUpdate++
}

func (l *LossRateController) OnPacketLost() {
	l.packetsSinceLastUpdate++
	l.lostSinceLastUpdate++
}

func (l *LossRateController) Update(lastDeliveryRate int) int {
	lossRate := float64(l.lostSinceLastUpdate) / float64(l.packetsSinceLastUpdate)
	var target float64
	if lossRate > 0.1 {
		target = float64(l.bitrate) * (1 - 0.5*lossRate)
		target = max(target, l.min)
	} else if lossRate < 0.02 {
		target = float64(l.bitrate) * 1.05
		target = max(min(target, 1.5*float64(lastDeliveryRate)), float64(l.bitrate))
		target = min(target, l.max)
	}
	if target != 0 {
		l.bitrate = int(target)
	}

	l.packetsSinceLastUpdate = 0
	l.arrivedSinceLastUpdate = 0
	l.lostSinceLastUpdate = 0

	return l.bitrate
}
