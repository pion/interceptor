//+build scream

package scream

import (
	"sync"
	"time"

	"github.com/mengelbart/scream-go"
	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RTPQueue implements the packet queue which will be used by SCReAM to buffer packets
type RTPQueue interface {
	scream.RTPQueue
	// Enqueue adds a new packet to the end of the queue.
	Enqueue(packet *rtp.Packet, ts uint64)
	// Dequeue removes and returns the first packet in the queue.
	Dequeue() *rtp.Packet
}

type localStream struct {
	queue       RTPQueue
	newFrame    chan struct{}
	newFeedback chan struct{}
	close       chan struct{}
}

// SenderInterceptor performs SCReAM congestion control
type SenderInterceptor struct {
	interceptor.NoOp
	m     sync.Mutex
	wg    sync.WaitGroup
	tx    *scream.Tx
	close chan struct{}
	log   logging.LeveledLogger

	newRTPQueue  func() RTPQueue
	rtpStreams   map[uint32]*localStream
	rtpStreamsMu sync.Mutex
}

// NewSenderInterceptor returns a new SenderInterceptor
func NewSenderInterceptor(opts ...SenderOption) (*SenderInterceptor, error) {
	s := &SenderInterceptor{
		tx:          scream.NewTx(),
		close:       make(chan struct{}),
		log:         logging.NewDefaultLoggerFactory().NewLogger("scream_sender"),
		newRTPQueue: newQueue,
		rtpStreams:  map[uint32]*localStream{},
	}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (s *SenderInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		n, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}
		pkts, err := rtcp.Unmarshal(b[:n])
		if err != nil {
			return 0, nil, err
		}
		for _, rtcpPacket := range pkts {
			s.m.Lock()
			s.tx.IncomingStandardizedFeedback(ntpTime(time.Now()), b[:n])
			s.m.Unlock()

			for _, ssrc := range rtcpPacket.DestinationSSRC() {
				s.rtpStreamsMu.Lock()
				s.rtpStreams[ssrc].newFeedback <- struct{}{}
				s.rtpStreamsMu.Unlock()
			}
		}

		return n, attr, nil
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (s *SenderInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	if !streamSupportSCReAM(info) {
		return writer
	}

	s.m.Lock()
	defer s.m.Unlock()

	if s.isClosed() {
		return writer
	}

	s.wg.Add(1)

	rtpQueue := s.newRTPQueue()
	localStream := &localStream{
		queue:       rtpQueue,
		newFrame:    make(chan struct{}, 1024), // TODO: remove hardcoded limit?
		newFeedback: make(chan struct{}, 1024), // TODO: remove hardcoded limit?
	}
	s.rtpStreamsMu.Lock()
	s.rtpStreams[info.SSRC] = localStream
	s.rtpStreamsMu.Unlock()

	// TODO: Somehow set these attributes per stream
	priority := float64(1)               // highest priority
	minBitrate := float64(1_000)         // 1Kbps
	startBitrate := float64(1_000)       // 1Kbps
	maxBitrate := float64(1_000_000_000) // 1Mbps

	s.tx.RegisterNewStream(rtpQueue, info.SSRC, priority, minBitrate, startBitrate, maxBitrate)

	go s.loop(writer, info.SSRC)

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		t := ntpTime(time.Now())
		pkt := &rtp.Packet{Header: *header, Payload: payload}

		// TODO: should attributes be stored in the queue, so we can pass them on later (see below)?
		rtpQueue.Enqueue(pkt, t)
		size := pkt.MarshalSize()
		s.m.Lock()
		s.tx.NewMediaFrame(t, header.SSRC, size)
		s.m.Unlock()
		localStream.newFeedback <- struct{}{}
		return size, nil
	})
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (s *SenderInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	s.rtpStreamsMu.Lock()
	defer s.rtpStreamsMu.Unlock()
	close(s.rtpStreams[info.SSRC].close)
	delete(s.rtpStreams, info.SSRC)
}

// Close closes the interceptor
func (s *SenderInterceptor) Close() error {
	defer s.wg.Wait()
	s.m.Lock()
	defer s.m.Unlock()

	if !s.isClosed() {
		close(s.close)
	}
	return nil
}

func (s *SenderInterceptor) loop(writer interceptor.RTPWriter, ssrc uint32) {
	s.rtpStreamsMu.Lock()
	stream := s.rtpStreams[ssrc]
	s.rtpStreamsMu.Unlock()

	// This is a bit ugly, because we need a timer so that case <-timer.C doesn't segfault.
	// However, that case only applies after the timer has explicitly been set in one
	// of the other cases first. So the timer will be disabled by default in the beginning.
	// The algorithm implemented here is documented in detail (a flow chart) here:
	// https://github.com/EricssonResearch/scream/blob/master/SCReAM-description.pptx
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	timerRunning := false
	defer s.log.Infof("leave send loop for ssrc: %v", ssrc)

	for {
		select {
		case <-timer.C:
			timerRunning = false
		case <-stream.newFrame:
			if timerRunning {
				continue
			}
		case <-stream.newFeedback:
			if timerRunning {
				continue
			}
		case <-s.close:
			return
		}

		if stream.queue.SizeOfQueue() <= 0 {
			continue
		}

		s.m.Lock()
		transmit := s.tx.IsOkToTransmit(ntpTime(time.Now()), ssrc)
		s.m.Unlock()
		switch {
		case transmit == -1:
			// no packets or CWND too small
			continue

		case transmit <= 1e-3:
			// send packet
			packet := stream.queue.Dequeue()
			if packet == nil {
				continue
			}
			t := time.Now()
			// TODO: Forward attributes from above?
			if _, err := writer.Write(&packet.Header, packet.Payload, interceptor.Attributes{}); err != nil {
				s.log.Warnf("failed sending RTP packet: %+v", err)
			}
			s.m.Lock()
			// TODO: set isMark?
			transmit = s.tx.AddTransmitted(ntpTime(t), ssrc, packet.MarshalSize(), packet.SequenceNumber, false)
			s.m.Unlock()
			if transmit == -1 {
				continue
			}
		}
		// Set timer: transmit is float in seconds, i.e. 0.123 seconds.
		timer = time.NewTimer(time.Duration(transmit*1000.0) * time.Millisecond)
		timerRunning = true
	}
}

// GetTargetBitrate returns the target bitrate calculated by SCReAM in bps.
func (s *SenderInterceptor) GetTargetBitrate(ssrc uint32) int64 {
	s.m.Lock()
	defer s.m.Unlock()
	return s.tx.GetTargetBitrate(ssrc)
}

func (s *SenderInterceptor) isClosed() bool {
	select {
	case <-s.close:
		return true
	default:
		return false
	}
}
