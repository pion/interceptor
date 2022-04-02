package nada

import (
	"log"
	"testing"
	"time"
)

func TestSenderReceiver_Simple(t *testing.T) {
	t0 := time.Now()
	sender := NewSender(t0, DefaultConfig())
	receiver := NewReceiver(t0, DefaultConfig())

	// send some data at 1 Mbps.
	seq := uint16(0)
	for ; seq < uint16(2_000); seq++ {
		t1 := t0.Add(time.Duration(seq) * time.Millisecond)
		t2 := t1.Add(25 * time.Millisecond)
		if err := receiver.OnReceiveMediaPacket(t2, t1, seq, false, 1000); err != nil {
			t.Fatal(err)
		}

		if seq%100 == 0 {
			report := receiver.BuildFeedbackReport()
			log.Printf("%v", report)
			sender.OnReceiveFeedbackReport(t2, report)

			// get the estimated bandwidth.
			log.Printf("%d %v %v", seq, sender.GetSendingRate(0), sender.GetTargetRate(0))
		}
	}

	// then introduce 25% loss.
	for ; seq < uint16(4_000); seq++ {
		t1 := t0.Add(time.Duration(seq) * time.Millisecond)
		t2 := t1.Add(25 * time.Millisecond)
		if seq%4 != 0 {
			if err := receiver.OnReceiveMediaPacket(t2, t1, seq, false, 1000); err != nil {
				t.Fatal(err)
			}
		}

		if seq%100 == 0 {
			report := receiver.BuildFeedbackReport()
			log.Printf("%v", report)
			sender.OnReceiveFeedbackReport(t2, report)

			// get the estimated bandwidth.
			log.Printf("%d %v %v", seq, sender.GetSendingRate(0), sender.GetTargetRate(0))
		}
	}
}
