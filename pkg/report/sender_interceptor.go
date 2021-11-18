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

// NewSenderInterceptor constructs a new SenderInterceptor
func NewSenderInterceptor(md *feature.MediaDescriptionReceiver, opts ...SenderOption) (*SenderInterceptor, error) {
	i := &SenderInterceptor{
		md:       md,
		interval: 1 * time.Second,
		now:      time.Now,
		streams:  make(map[uint32]*senderStream),
		log:      logging.NewDefaultLoggerFactory().NewLogger("sender_interceptor"),
		close:    make(chan struct{}),
	}

	for _, opt := range opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	return i, nil
}

// SenderInterceptor interceptor generates sender reports.
type SenderInterceptor struct {
	md       *feature.MediaDescriptionReceiver
	interval time.Duration
	now      func() time.Time
	streams  map[uint32]*senderStream
	log      logging.LeveledLogger
	m        sync.Mutex
	close    chan struct{}
}

// Close closes the interceptor.
func (i *SenderInterceptor) Close() error {
	close(i.close)

	return nil
}

// Transform transforms a given set of sender interceptor pipes.
func (i *SenderInterceptor) Transform(rtpSink rtpio.RTPWriter, rtcpSink rtpio.RTCPWriter, rtcpSrc rtpio.RTCPReader) rtpio.RTPWriter {
	go i.loop(rtcpSink)
	s := &senderRTPWriter{
		interceptor: i,
		rtpSink:     rtpSink,
	}
	return s
}

type senderRTPWriter struct {
	interceptor *SenderInterceptor
	rtpSink     rtpio.RTPWriter
}

func (s *senderRTPWriter) WriteRTP(pkt *rtp.Packet) (int, error) {
	s.interceptor.m.Lock()
	stream, ok := s.interceptor.streams[pkt.SSRC]
	if !ok {
		clockRate, ok := s.interceptor.md.GetClockRate(pkt.SSRC, pkt.PayloadType)
		if !ok {
			// we don't have information about this clock rate
			s.interceptor.m.Unlock()
			return s.rtpSink.WriteRTP(pkt)
		}
		stream = newSenderStream(clockRate)
		s.interceptor.streams[pkt.SSRC] = stream
	}
	s.interceptor.m.Unlock()

	stream.processRTP(s.interceptor.now(), &pkt.Header, pkt.Payload)

	return s.rtpSink.WriteRTP(pkt)
}

func (i *SenderInterceptor) loop(rtcpWriter rtpio.RTCPWriter) {
	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := i.now()
			for ssrc, stream := range i.streams {
				stream.m.Lock()
				defer stream.m.Unlock()

				sr := &rtcp.SenderReport{
					SSRC:        ssrc,
					NTPTime:     ntpTime(now),
					RTPTime:     stream.lastRTPTimeRTP + uint32(now.Sub(stream.lastRTPTimeTime).Seconds()*stream.clockRate),
					PacketCount: stream.packetCount,
					OctetCount:  stream.octetCount,
				}

				if _, err := rtcpWriter.WriteRTCP([]rtcp.Packet{sr}); err != nil {
					i.log.Warnf("failed sending: %+v", err)
				}
			}

		case <-i.close:
			return
		}
	}
}

func ntpTime(t time.Time) uint64 {
	// seconds since 1st January 1900
	s := (float64(t.UnixNano()) / 1000000000) + 2208988800

	// higher 32 bits are the integer part, lower 32 bits are the fractional part
	integerPart := uint32(s)
	fractionalPart := uint32((s - float64(integerPart)) * 0xFFFFFFFF)
	return uint64(integerPart)<<32 | uint64(fractionalPart)
}
