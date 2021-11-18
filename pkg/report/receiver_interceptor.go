package report

import (
	"sync"
	"time"

	"github.com/pion/interceptor/v2/pkg/feature"
	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// NewReceiverInterceptor constructs a new ReceiverInterceptor
func NewReceiverInterceptor(md *feature.MediaDescriptionReceiver, opts ...ReceiverOption) (*ReceiverInterceptor, error) {
	i := &ReceiverInterceptor{
		md:       md,
		interval: 1 * time.Second,
		now:      time.Now,
		streams:  make(map[uint32]*receiverStream),
		log:      logging.NewDefaultLoggerFactory().NewLogger("receiver_interceptor"),
		close:    make(chan struct{}),
	}

	for _, opt := range opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	return i, nil
}

// ReceiverInterceptor interceptor generates receiver reports.
type ReceiverInterceptor struct {
	md       *feature.MediaDescriptionReceiver
	interval time.Duration
	now      func() time.Time
	streams  map[uint32]*receiverStream
	log      logging.LeveledLogger
	m        sync.Mutex
	close    chan struct{}
}

// Close closes the interceptor.
func (i *ReceiverInterceptor) Close() error {
	close(i.close)

	return nil
}

// Transform transforms a given set of receiver interceptor pipes.
func (i *ReceiverInterceptor) Transform(rtcpSink rtpio.RTCPWriter, rtpSrc rtpio.RTPReader, rtcpSrc rtpio.RTCPReader) rtpio.RTPReader {
	if rtcpSrc != nil {
		go i.processRTCP(rtcpSrc)
	}
	go i.loop(rtcpSink)

	r := &receiverRTPReader{
		interceptor: i,
		rtpSrc:      rtpSrc,
	}
	return r
}

type receiverRTPReader struct {
	interceptor *ReceiverInterceptor
	rtpSrc      rtpio.RTPReader
}

func (r *receiverRTPReader) ReadRTP(pkt *rtp.Packet) (int, error) {
	i, err := r.rtpSrc.ReadRTP(pkt)
	if err != nil {
		return 0, err
	}

	r.interceptor.m.Lock()
	stream, ok := r.interceptor.streams[pkt.SSRC]
	if !ok {
		clockRate, ok := r.interceptor.md.GetClockRate(pkt.SSRC, pkt.PayloadType)
		if !ok {
			// we don't have information about this clock rate
			r.interceptor.m.Unlock()
			return i, nil
		}
		stream = newReceiverStream(pkt.SSRC, clockRate)
		r.interceptor.streams[pkt.SSRC] = stream
	}
	r.interceptor.m.Unlock()

	stream.processRTP(r.interceptor.now(), &pkt.Header)

	return i, nil
}

func (i *ReceiverInterceptor) loop(rtcpWriter rtpio.RTCPWriter) {
	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := i.now()
			i.m.Lock()
			for _, stream := range i.streams {
				var pkts []rtcp.Packet

				pkts = append(pkts, stream.generateReport(now))

				if _, err := rtcpWriter.WriteRTCP(pkts); err != nil {
					i.log.Warnf("failed sending: %+v", err)
				}
			}
			i.m.Unlock()

		case <-i.close:
			return
		}
	}
}

func (i *ReceiverInterceptor) processRTCP(reader rtpio.RTCPReader) {
	for {
		pkts := make([]rtcp.Packet, 16)
		_, err := reader.ReadRTCP(pkts)
		if err != nil {
			return
		}

		for _, pkt := range pkts {
			if sr, ok := (pkt).(*rtcp.SenderReport); ok {
				stream, ok := i.streams[sr.SSRC]
				if !ok {
					continue
				}

				stream.processSenderReport(i.now(), sr)
			}
		}
	}
}
