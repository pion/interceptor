// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

var errLeakyBucketPacerPoolCastFailed = errors.New("failed to access leaky bucket pacer pool, cast failed")

type item struct {
	header     *rtp.Header
	payload    *[]byte
	size       int
	attributes interceptor.Attributes
}

// LeakyBucketPacer implements a leaky bucket pacing algorithm
type LeakyBucketPacer struct {
	log logging.LeveledLogger

	f                 float64
	targetBitrate     int
	targetBitrateLock sync.Mutex

	pacingInterval time.Duration

	qLock sync.RWMutex
	queue *list.List
	done  chan struct{}

	ssrcToWriter map[uint32]interceptor.RTPWriter
	writerLock   sync.RWMutex

	pool *sync.Pool
}

// NewLeakyBucketPacer initializes a new LeakyBucketPacer
func NewLeakyBucketPacer(initialBitrate int) *LeakyBucketPacer {
	p := &LeakyBucketPacer{
		log:            logging.NewDefaultLoggerFactory().NewLogger("pacer"),
		f:              1.5,
		targetBitrate:  initialBitrate,
		pacingInterval: 5 * time.Millisecond,
		qLock:          sync.RWMutex{},
		queue:          list.New(),
		done:           make(chan struct{}),
		ssrcToWriter:   map[uint32]interceptor.RTPWriter{},
		pool:           &sync.Pool{},
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

// AddStream adds a new stream and its corresponding writer to the pacer
func (p *LeakyBucketPacer) AddStream(ssrc uint32, writer interceptor.RTPWriter) {
	p.writerLock.Lock()
	defer p.writerLock.Unlock()
	p.ssrcToWriter[ssrc] = writer
}

// SetTargetBitrate updates the target bitrate at which the pacer is allowed to
// send packets. The pacer may exceed this limit by p.f
func (p *LeakyBucketPacer) SetTargetBitrate(rate int) {
	p.targetBitrateLock.Lock()
	defer p.targetBitrateLock.Unlock()
	p.targetBitrate = int(p.f * float64(rate))
}

func (p *LeakyBucketPacer) getTargetBitrate() int {
	p.targetBitrateLock.Lock()
	defer p.targetBitrateLock.Unlock()

	return p.targetBitrate
}

// Write sends a packet with header and payload the a previously registered
// stream.
func (p *LeakyBucketPacer) Write(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
	buf, ok := p.pool.Get().(*[]byte)
	if !ok {
		return 0, errLeakyBucketPacerPoolCastFailed
	}

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
	defer ticker.Stop()

	lastSent := time.Now()
	for {
		select {
		case <-p.done:
			return
		case now := <-ticker.C:
			budget := int(float64(now.Sub(lastSent).Milliseconds()) * float64(p.getTargetBitrate()) / 8000.0)
			p.qLock.Lock()
			for p.queue.Len() != 0 && budget > 0 {
				p.log.Infof("budget=%v, len(queue)=%v, targetBitrate=%v", budget, p.queue.Len(), p.getTargetBitrate())
				next, ok := p.queue.Remove(p.queue.Front()).(*item)
				p.qLock.Unlock()
				if !ok {
					p.log.Warnf("failed to access leaky bucket pacer queue, cast failed")
					continue
				}

				p.writerLock.RLock()
				writer, ok := p.ssrcToWriter[next.header.SSRC]
				p.writerLock.RUnlock()
				if !ok {
					p.log.Warnf("no writer found for ssrc: %v", next.header.SSRC)
					p.pool.Put(next.payload)
					p.qLock.Lock()
					continue
				}

				n, err := writer.Write(next.header, (*next.payload)[:next.size], next.attributes)
				if err != nil {
					p.log.Errorf("failed to write packet: %v", err)
				}
				lastSent = now
				budget -= n

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
