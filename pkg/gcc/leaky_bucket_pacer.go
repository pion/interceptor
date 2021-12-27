package gcc

import (
	"container/list"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

type item struct {
	header     *rtp.Header
	payload    *[]byte
	size       int
	attributes interceptor.Attributes
}

// LeakyBucketPacer implements a leaky bucket pacing algorithm
type LeakyBucketPacer struct {
	log logging.LeveledLogger

	f              float64
	targetBitrate  int
	pacingInterval time.Duration

	qLock     sync.RWMutex
	queue     *list.List
	bitrateCh chan int
	streamCh  chan stream
	done      chan struct{}

	ssrcToWriter map[uint32]interceptor.RTPWriter

	pool *sync.Pool
}

// NewLeakyBucketPacer initializes a new LeakyBucketPacer
func NewLeakyBucketPacer(initialBitrate int) *LeakyBucketPacer {
	p := &LeakyBucketPacer{
		log:            logging.NewDefaultLoggerFactory().NewLogger("pacer"),
		f:              1.5,
		targetBitrate:  initialBitrate,
		pacingInterval: 5 * time.Millisecond,
		queue:          list.New(),
		bitrateCh:      make(chan int),
		streamCh:       make(chan stream),
		done:           make(chan struct{}),
		ssrcToWriter:   map[uint32]interceptor.RTPWriter{},
	}
	p.pool = &sync.Pool{
		New: func() interface{} {
			b := make([]byte, 1460)
			return &b
		},
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
	buf := p.pool.Get().(*[]byte)
	copy(*buf, payload)
	hdr := header.Clone()

	p.qLock.Lock()
	p.queue.PushBack(&item{
		header:     &hdr,
		payload:    buf,
		size:       len(payload),
		attributes: attributes,
	})
	p.qLock.Unlock()

	return header.MarshalSize() + len(payload), nil
}

// Run starts the LeakyBucketPacer
func (p *LeakyBucketPacer) Run() {
	ticker := time.NewTicker(p.pacingInterval)

	for {
		select {
		case <-p.done:
			return
		case rate := <-p.bitrateCh:
			p.targetBitrate = rate
		case stream := <-p.streamCh:
			p.ssrcToWriter[stream.ssrc] = stream.writer
		case <-ticker.C:
			budget := p.pacingInterval.Milliseconds() * int64(float64(p.targetBitrate)/8000.0)
			p.qLock.Lock()
			for p.queue.Len() != 0 && budget > 0 {
				p.log.Infof("pacer budget=%v, len(queue)=%v", budget, p.queue.Len())
				next := p.queue.Remove(p.queue.Front()).(*item)
				p.qLock.Unlock()

				writer, ok := p.ssrcToWriter[next.header.SSRC]
				if !ok {
					p.log.Warnf("no writer found for ssrc: %v", next.header.SSRC)
					p.pool.Put(next.payload)
					p.qLock.Lock()
					continue
				}

				n, err := writer.Write(next.header, (*next.payload)[:next.size], next.attributes)
				p.log.Infof("pacer sent %v bytes", n)
				if err != nil {
					p.log.Errorf("failed to write packet: %v", err)
				}

				budget -= int64(n)

				p.pool.Put(next.payload)
				p.qLock.Lock()
			}
			p.qLock.Unlock()
		}
	}
}

// Close closes the LeakyBucketPacer
func (p *LeakyBucketPacer) Close() error {
	close(p.done)
	return nil
}
