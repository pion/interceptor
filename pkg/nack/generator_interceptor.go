package nack

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// GeneratorInterceptor interceptor generates nack feedback messages.
type GeneratorInterceptor struct {
	size      uint16
	skipLastN uint16
	interval  time.Duration
	close     chan struct{}
	log       logging.LeveledLogger

	receiveLogs   map[uint32]*receiveLog
	receiveLogsMu sync.Mutex
}

// NewGeneratorInterceptor returns a new GeneratorInterceptorFactory
func NewGeneratorInterceptor(opts ...GeneratorOption) (*GeneratorInterceptor, error) {
	g := &GeneratorInterceptor{
		size:        8192,
		skipLastN:   0,
		interval:    time.Millisecond * 100,
		receiveLogs: map[uint32]*receiveLog{},
		close:       make(chan struct{}),
		log:         logging.NewDefaultLoggerFactory().NewLogger("nack_generator"),
	}
	for _, opt := range opts {
		if err := opt(g); err != nil {
			return nil, err
		}
	}
	allowedSizes := make([]uint16, 0)
	correctSize := false
	for i := 6; i < 16; i++ {
		if g.size == 1<<i {
			correctSize = true
			break
		}
		allowedSizes = append(allowedSizes, 1<<i)
	}

	if !correctSize {
		return nil, fmt.Errorf("%w: %d is not a valid size, allowed sizes: %v", ErrInvalidSize, g.size, allowedSizes)
	}
	return g, nil
}

// Transform transforms a given set of receiver interceptor pipes.
func (n *GeneratorInterceptor) Transform(rtcpSink rtpio.RTCPWriter, rtpSrc rtpio.RTPReader, rtcpSrc rtpio.RTCPReader) rtpio.RTPReader {
	go n.loop(rtcpSink)

	// by contract we must consume all the srcs.
	go rtpio.ConsumeRTCP(rtcpSrc)

	r := &generatorRTPReader{
		interceptor: n,
		rtpSrc:      rtpSrc,
	}
	return r
}

type generatorRTPReader struct {
	interceptor *GeneratorInterceptor
	rtpSrc      rtpio.RTPReader
}

// ReadRTP pulls the next RTP packet from the source.
func (g *generatorRTPReader) ReadRTP(pkt *rtp.Packet) (int, error) {
	g.interceptor.receiveLogsMu.Lock()
	log, ok := g.interceptor.receiveLogs[pkt.SSRC]
	if !ok {
		log = newReceiveLog(g.interceptor.size)
		g.interceptor.receiveLogs[pkt.SSRC] = log
	}
	g.interceptor.receiveLogsMu.Unlock()

	i, err := g.rtpSrc.ReadRTP(pkt)
	if err != nil {
		return 0, err
	}

	log.add(pkt.Header.SequenceNumber)

	return i, nil
}

func (n *GeneratorInterceptor) loop(rtcpWriter rtpio.RTCPWriter) {
	senderSSRC := rand.Uint32() // #nosec

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n.receiveLogsMu.Lock()

			for ssrc, receiveLog := range n.receiveLogs {
				missing := receiveLog.missingSeqNumbers(n.skipLastN)
				if len(missing) == 0 {
					continue
				}

				nack := &rtcp.TransportLayerNack{
					SenderSSRC: senderSSRC,
					MediaSSRC:  ssrc,
					Nacks:      rtcp.NackPairsFromSequenceNumbers(missing),
				}

				if _, err := rtcpWriter.WriteRTCP([]rtcp.Packet{nack}); err != nil {
					n.log.Warnf("failed sending nack: %+v", err)
				}
			}

			n.receiveLogsMu.Unlock()
		case <-n.close:
			return
		}
	}
}

// Close closes the interceptor for reading/writing.
func (n *GeneratorInterceptor) Close() error {
	close(n.close)
	return nil
}
