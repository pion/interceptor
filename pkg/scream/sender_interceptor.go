package scream

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mengelbart/scream-go"
	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type BandwidthEstimator interface {
	GetTargetBitrate(ssrc uint32) (int, error)
	GetStats() map[string]interface{}
}

type NewPeerConnectionCallback func(id string, estimator BandwidthEstimator)

// RTPQueue implements the packet queue which will be used by SCReAM to buffer packets
type RTPQueue interface {
	scream.RTPQueue
	// Enqueue adds a new packet to the end of the queue.
	Enqueue(packet *rtp.Packet, ts float64)
	// Dequeue removes and returns the first packet in the queue.
	Dequeue() *rtp.Packet
}

type localStream struct {
	queue       RTPQueue
	newFrame    chan struct{}
	newFeedback chan struct{}
	close       chan struct{}
}

type SenderInterceptorFactory struct {
	opts              []SenderOption
	addPeerConnection NewPeerConnectionCallback
}

func NewSenderInterceptor(opts ...SenderOption) (*SenderInterceptorFactory, error) {
	return &SenderInterceptorFactory{
		opts: opts,
	}, nil
}

func (f *SenderInterceptorFactory) OnNewPeerConnection(cb NewPeerConnectionCallback) {
	f.addPeerConnection = cb
}

func (f *SenderInterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	s := &SenderInterceptor{
		NoOp:           interceptor.NoOp{},
		m:              sync.Mutex{},
		wg:             sync.WaitGroup{},
		tx:             scream.NewTx(),
		close:          make(chan struct{}),
		log:            logging.NewDefaultLoggerFactory().NewLogger("scream_sender"),
		newRTPQueue:    newQueue,
		rtpStreams:     map[uint32]*localStream{},
		rtpStreamsMu:   sync.Mutex{},
		minBitrate:     1_000,
		initialBitrate: 100_000,
		maxBitrate:     2048000000,
		t0:             getNTPT0(),
	}
	for _, opt := range f.opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	if f.addPeerConnection != nil {
		f.addPeerConnection(id, s)
	}
	return s, nil
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

	minBitrate     float64
	initialBitrate float64
	maxBitrate     float64

	t0 float64
}

func (s *SenderInterceptor) getTimeNTP(t time.Time) uint64 {
	return getTimeBetweenNTP(s.t0, t)
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (s *SenderInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		n, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}
		buf := make([]byte, n)
		copy(buf, b)
		pkts, err := rtcp.Unmarshal(buf)
		if err != nil {
			return 0, nil, err
		}

		t := s.getTimeNTP(time.Now())
		for _, pkt := range pkts {
			packet, ok := pkt.(*rtcp.RawPacket)
			if !ok {
				s.log.Info("got incorrect packet type, skipping feedback")
				continue
			}

			s.m.Lock()
			s.tx.IncomingStandardizedFeedback(t, b[:n])
			s.m.Unlock()

			ssrcs := extractSSRCs(*packet)

			for _, ssrc := range ssrcs {
				s.rtpStreamsMu.Lock()
				if stream, ok := s.rtpStreams[ssrc]; ok {
					stream.newFeedback <- struct{}{}
				}
				s.rtpStreamsMu.Unlock()
			}
		}

		return n, attr, nil
	})
}

func extractSSRCs(packet []byte) []uint32 {
	uniqueSSRCs := make(map[uint32]struct{})
	var ssrcs []uint32

	offset := 8
	for offset < len(packet)-4 {
		ssrc := binary.BigEndian.Uint32(packet[offset:])

		if _, ok := uniqueSSRCs[ssrc]; !ok {
			ssrcs = append(ssrcs, ssrc)
			uniqueSSRCs[ssrc] = struct{}{}
		}

		numReports := binary.BigEndian.Uint16(packet[offset+6:])

		// pad 16 bits 0 if numReports is not a multiple of 2
		if numReports%2 != 0 {
			numReports++
		}
		offset += 2 * int(numReports) // 2 bytes per report
		offset += 8                   // 4 byte SSRC + 2 bytes begin_seq + 2 bytes num_reports
	}

	return ssrcs
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (s *SenderInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {

	s.m.Lock()
	defer s.m.Unlock()

	if s.isClosed() {
		return writer
	}

	s.wg.Add(1)

	rtpQueue := s.newRTPQueue()
	localStream := &localStream{
		queue:       rtpQueue,
		newFrame:    make(chan struct{}),
		newFeedback: make(chan struct{}),
	}
	s.rtpStreamsMu.Lock()
	s.rtpStreams[info.SSRC] = localStream
	s.rtpStreamsMu.Unlock()

	// TODO: Somehow set these attributes per stream
	priority := float64(1) // highest priority
	minBitrate := s.minBitrate
	startBitrate := s.initialBitrate
	maxBitrate := s.maxBitrate

	s.tx.RegisterNewStream(rtpQueue, info.SSRC, priority, minBitrate, startBitrate, maxBitrate)

	go s.loop(writer, info.SSRC)

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		t := s.getTimeNTP(time.Now())

		buf := make([]byte, len(payload))
		copy(buf, payload)
		pkt := &rtp.Packet{Header: header.Clone(), Payload: buf}

		// TODO: should attributes be stored in the queue, so we can pass them on later (see below)?
		rtpQueue.Enqueue(pkt, float64(t)/65536.0)
		size := pkt.MarshalSize()
		s.m.Lock()
		//fmt.Printf("newMediaFrame at t=%v\n", t)
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
	defer s.wg.Done()
	s.rtpStreamsMu.Lock()
	stream := s.rtpStreams[ssrc]
	s.rtpStreamsMu.Unlock()

	defer s.log.Infof("leave send loop for ssrc: %v", ssrc)

	for {
		select {
		case <-stream.newFrame:
		case <-stream.newFeedback:
		case <-s.close:
			return
		default:
		}

		if stream.queue.SizeOfQueue() <= 0 {
			continue
		}

		s.m.Lock()
		transmit := s.tx.IsOkToTransmit(s.getTimeNTP(time.Now()), ssrc)
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
			// TODO: Forward attributes from above?
			if _, err := writer.Write(&packet.Header, packet.Payload, interceptor.Attributes{}); err != nil {
				s.log.Warnf("failed sending RTP packet: %+v", err)
			}
			s.m.Lock()
			s.tx.AddTransmitted(s.getTimeNTP(time.Now()), ssrc, packet.MarshalSize(), packet.SequenceNumber, packet.Marker)
			s.m.Unlock()
		}
	}
}

// GetTargetBitrate returns the target bitrate calculated by SCReAM in bps.
func (s *SenderInterceptor) GetTargetBitrate(ssrc uint32) (int, error) {
	s.rtpStreamsMu.Lock()
	_, ok := s.rtpStreams[ssrc]
	s.rtpStreamsMu.Unlock()
	if !ok {
		return 0, fmt.Errorf("unknown SSRC, the stream may be unsupported")
	}

	s.m.Lock()
	defer s.m.Unlock()
	return int(s.tx.GetTargetBitrate(ssrc)), nil
}

func (s *SenderInterceptor) GetStats() map[string]interface{} {
	stats := s.tx.GetStatistics(s.getTimeNTP(time.Now()) / 65536.0)
	statSlice := strings.Split(stats, ",")
	keys := []string{
		"queueDelay",
		"queueDelayMax",
		"queueDelayMinAvg",
		"sRTT",
		"cwnd",
		"bytesInFlightLog",
		"rateTransmittedConnection",
		"isInFastStart",
		"rtpQueueDelayStream0",
		"targetBitrateStream0",
		"rateRTPStream0",
		"rateTransmittedStream0",
		"rateAckedStream0",
		"rateLostStream0",
		"rateCEStream0",
		"hiSeqAckStream0",
	}
	res := make(map[string]interface{})
	for i := 0; i < len(statSlice) && i < len(keys); i++ {
		val := strings.TrimSpace(statSlice[i])
		res[keys[i]] = val
	}
	return res
}

func (s *SenderInterceptor) isClosed() bool {
	select {
	case <-s.close:
		return true
	default:
		return false
	}
}
