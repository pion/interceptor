package bwe

import "time"

func MeasureRTT(reportSent, reportReceived, latestAckedSent, latestAckedArrival time.Time) time.Duration {
	pendingTime := reportSent.Sub(latestAckedArrival)
	return reportReceived.Sub(latestAckedSent) - pendingTime
}
