package cc

import (
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/types"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

type item struct {
	header     *rtp.Header
	payload    []byte
	attributes interceptor.Attributes
}

type LeakyBucketPacer struct {
	log logging.LeveledLogger

	targetBitrate  types.DataRate
	pacingInterval time.Duration

	itemCh    chan item
	bitrateCh chan types.DataRate
	streamCh  chan stream
	done      chan struct{}

	ssrcToWriter map[uint32]interceptor.RTPWriter
}

func NewLeakyBucketPacer() *LeakyBucketPacer {
	p := &LeakyBucketPacer{
		log:            logging.NewDefaultLoggerFactory().NewLogger("pacer"),
		targetBitrate:  types.DataRate(150_000),
		pacingInterval: 5 * time.Millisecond,
		itemCh:         make(chan item),
		bitrateCh:      make(chan types.DataRate),
		streamCh:       make(chan stream),
		done:           make(chan struct{}),
		ssrcToWriter:   map[uint32]interceptor.RTPWriter{},
	}
	go p.Run()
	return p
}

type stream struct {
	ssrc   uint32
	writer interceptor.RTPWriter
}

func (p *LeakyBucketPacer) AddStream(ssrc uint32, writer interceptor.RTPWriter) {
	p.streamCh <- stream{
		ssrc:   ssrc,
		writer: writer,
	}
}

func (p *LeakyBucketPacer) SetTargetBitrate(rate types.DataRate) {
	p.targetBitrate = rate
}

func (p *LeakyBucketPacer) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	p.itemCh <- item{
		header:     header,
		payload:    payload,
		attributes: attributes,
	}
	return header.MarshalSize() + len(payload), nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (p *LeakyBucketPacer) Run() {
	ticker := time.NewTicker(p.pacingInterval)

	queue := []item{}

	for {
		select {
		case <-p.done:
			return
		case rate := <-p.bitrateCh:
			p.targetBitrate = rate
		case stream := <-p.streamCh:
			p.ssrcToWriter[stream.ssrc] = stream.writer
		case item := <-p.itemCh:
			queue = append(queue, item)
		case <-ticker.C:
			budget := p.pacingInterval.Milliseconds() * int64(p.targetBitrate.BitsPerMillisecond())

			for len(queue) != 0 && budget > 0 {
				p.log.Infof("pacer budget=%v, len(queue)=%v", budget, len(queue))
				next := queue[0]
				queue = queue[1:]
				writer, ok := p.ssrcToWriter[next.header.SSRC]
				if !ok {
					p.log.Infof("no writer found for ssrc: %v", next.header.SSRC)
				}
				var twcc rtp.TransportCCExtension
				ext := next.header.GetExtension(next.header.GetExtensionIDs()[0])
				if err := twcc.Unmarshal(ext); err != nil {
					panic(err)
				}
				p.log.Infof("pacer sending packet %v", twcc.TransportSequence)
				n, err := writer.Write(next.header, next.payload, next.attributes)
				if err != nil {
					p.log.Errorf("failed to write packet: %v", err)
				}
				budget -= int64(n)
			}
		}
	}
}

func (p *LeakyBucketPacer) Close() error {
	close(p.done)
	return nil
}
