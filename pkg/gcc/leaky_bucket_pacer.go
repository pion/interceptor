package gcc

import (
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

type item struct {
	header     *rtp.Header
	payload    []byte
	attributes interceptor.Attributes
}

// LeakyBucketPacer implements a leaky bucket pacing algorithm
type LeakyBucketPacer struct {
	log logging.LeveledLogger

	f              float64
	targetBitrate  int
	pacingInterval time.Duration

	itemCh    chan item
	bitrateCh chan int
	streamCh  chan stream
	done      chan struct{}

	ssrcToWriter map[uint32]interceptor.RTPWriter
}

// NewLeakyBucketPacer initializes a new LeakyBucketPacer
func NewLeakyBucketPacer(initialBitrate int) *LeakyBucketPacer {
	p := &LeakyBucketPacer{
		log:            logging.NewDefaultLoggerFactory().NewLogger("pacer"),
		f:              1.5,
		targetBitrate:  initialBitrate,
		pacingInterval: 5 * time.Millisecond,
		itemCh:         make(chan item),
		bitrateCh:      make(chan int),
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

// AddStream adds a new stream and its corresponding writer to the pacer
func (p *LeakyBucketPacer) AddStream(ssrc uint32, writer interceptor.RTPWriter) {
	p.streamCh <- stream{
		ssrc:   ssrc,
		writer: writer,
	}
}

// SetTargetBitrate updates the target bitrate at which the pacer is allowed to
// send packets. The pacer may exceed this limit by p.f
func (p *LeakyBucketPacer) SetTargetBitrate(rate int) {
	p.bitrateCh <- int(p.f * float64(rate))
}

// Write sends a packet with header and payload the a previously registered
// stream.
func (p *LeakyBucketPacer) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	p.itemCh <- item{
		header:     header,
		payload:    payload,
		attributes: attributes,
	}
	return header.MarshalSize() + len(payload), nil
}

// Run starts the LeakyBucketPacer
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
			budget := p.pacingInterval.Milliseconds() * int64(float64(p.targetBitrate)/1000.0)
			for len(queue) != 0 && budget > 0 {
				p.log.Infof("pacer budget=%v, len(queue)=%v", budget, len(queue))
				next := queue[0]
				queue = queue[1:]
				writer, ok := p.ssrcToWriter[next.header.SSRC]
				if !ok {
					p.log.Warnf("no writer found for ssrc: %v", next.header.SSRC)
					continue
				}
				n, err := writer.Write(next.header, next.payload, next.attributes)
				if err != nil {
					p.log.Errorf("failed to write packet: %v", err)
				}
				budget -= int64(n)
			}
		}
	}
}

// Close closes the LeakyBucketPacer
func (p *LeakyBucketPacer) Close() error {
	close(p.done)
	return nil
}
